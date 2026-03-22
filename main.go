package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"time"

	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"golang.org/x/crypto/ssh"
)

type State struct {
	State string `json:"state"`
}

var (
	mqttHost     = getEnv("mqtt_host", "")
	mqttUser     = getEnv("mqtt_user", "")
	mqttPass     = getEnv("mqtt_pass", "")
	sshHost      = getEnv("ssh_host", "")
	sshUser      = getEnv("ssh_user", "")
	sshPass      = getEnv("ssh_pass", "")
	moonrakerURL = getEnv("moonraker_url", "")
)

func getEnv(key, fallback string) string {
	_ = godotenv.Load(".env")
	_ = godotenv.Load("/root/power-api/.env")
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

func getCurrentExtruderTemperature() int {
	resp, err := http.Get(fmt.Sprintf("%s/server/temperature_store", moonrakerURL))
	if err != nil {
		return 0
	}
	defer resp.Body.Close()

	var result struct {
		Result struct {
			Extruder struct {
				Temperatures []float64 `json:"temperatures"`
			} `json:"extruder"`
		} `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 999
	}

	t := result.Result.Extruder.Temperatures
	if len(t) <= 10 {
		return 999
	}

	t = t[len(t)-10:]

	var filtered []float64
	for _, v := range t {
		if v >= 1 && v <= 400 {
			filtered = append(filtered, v)
		}
	}

	if len(filtered) <= 5 {
		return 999
	}

	sum := 0.0
	for _, v := range filtered {
		sum += v
	}

	avg := sum / float64(len(filtered))
	return int(avg)
}

func sendSSHCommand(cmd string) {
	config := &ssh.ClientConfig{
		User: sshUser,
		Auth: []ssh.AuthMethod{ssh.Password(sshPass)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:22", sshHost), config)
	if err != nil {
		return
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return
	}
	defer session.Close()

	_ = session.Run(cmd)
}

func ping(host string) bool {
	cmd := exec.Command("ping", "-c", "1", host)
	return cmd.Run() == nil
}

func subscribeAndGetStateWithContext(ctx context.Context, client MQTT.Client, topic string) (string, error) {
	messageChan := make(chan MQTT.Message)

	callback := func(_ MQTT.Client, msg MQTT.Message) {
		messageChan <- msg
	}

	if token := client.Subscribe(topic, 0, callback); token.Wait() && token.Error() != nil {
		return "", fmt.Errorf("error subscribing to topic: %v", token.Error())
	}
	defer func() {
		if token := client.Unsubscribe(topic); token.Wait() && token.Error() != nil {
			fmt.Printf("error unsubscribing from topic: %v\n", token.Error())
		}
	}()

	select {
	case msg := <-messageChan:
		var response struct {
			State string `json:"state"`
		}
		if err := json.Unmarshal(msg.Payload(), &response); err != nil {
			return "", fmt.Errorf("error parsing message payload: %v", err)
		}
		return response.State, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func setupMQTTClient() MQTT.Client {
	opts := MQTT.NewClientOptions().
		AddBroker(fmt.Sprintf("tcp://%s:1883", mqttHost)).
		SetUsername(mqttUser).
		SetPassword(mqttPass).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(5 * time.Second).
		SetMaxReconnectInterval(1 * time.Minute).
		SetKeepAlive(30 * time.Second).
		SetConnectionLostHandler(func(_ MQTT.Client, err error) {
			fmt.Printf("Connection lost: %v\n", err)
		}).
		SetOnConnectHandler(func(_ MQTT.Client) {
			fmt.Println("Connected to MQTT broker")
		}).
		SetReconnectingHandler(func(_ MQTT.Client, _ *MQTT.ClientOptions) {
			fmt.Println("Attempting to reconnect to MQTT broker...")
		})

	client := MQTT.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		fmt.Printf("Failed to connect to MQTT broker: %v\n", token.Error())
		for i := 0; i < 5; i++ {
			time.Sleep(time.Duration(i+1) * time.Second)
			if token := client.Connect(); token.Wait() && token.Error() == nil {
				break
			}
			fmt.Printf("Retry %d failed\n", i+1)
		}
		if !client.IsConnected() {
			panic("Failed to connect to MQTT broker after multiple attempts")
		}
	}

	return client
}

func ensureConnected(client MQTT.Client) error {
	if !client.IsConnected() {
		if token := client.Connect(); token.Wait() && token.Error() != nil {
			return fmt.Errorf("failed to reconnect to MQTT broker: %v", token.Error())
		}
	}
	return nil
}

func main() {
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	client := setupMQTTClient()
	defer client.Disconnect(250)

	r.GET("/api/3d-printer", func(c *gin.Context) {
		if err := ensureConnected(client); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "MQTT connection unavailable"})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		state, err := subscribeAndGetStateWithContext(ctx, client, "zigbee2mqtt/R")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, State{State: state})
	})

	r.POST("/api/3d-printer", func(c *gin.Context) {
		var reqBody State
		if err := c.BindJSON(&reqBody); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		switch reqBody.State {
		case "ON":
			token := client.Publish("zigbee2mqtt/R/set", 0, false, `{"state": "ON"}`)
			token.Wait()
			c.JSON(http.StatusOK, State{State: "ON"})
		case "OFF":
			for getCurrentExtruderTemperature() >= 49 {
				time.Sleep(5 * time.Second)
			}

			sendSSHCommand("/sbin/shutdown 0")

			for ping(sshHost) {
				time.Sleep(5 * time.Second)
			}

			time.Sleep(5 * time.Second)

			token := client.Publish("zigbee2mqtt/R/set", 0, false, `{"state": "OFF"}`)
			token.Wait()
			c.JSON(http.StatusOK, State{State: "OFF"})
		default:
			c.JSON(http.StatusOK, State{State: ""})
		}
	})

	r.Run(":8000")
}

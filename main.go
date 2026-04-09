package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"golang.org/x/crypto/ssh"
)

const (
	defaultThresholdTemp = 49
	mqttConnectTimeout   = 10 * time.Second
	mqttCommandTimeout   = 5 * time.Second
	sshCommandTimeout    = 10 * time.Second
	hostPollTimeout      = 30 * time.Second
	pollInterval         = 5 * time.Second
	serverPort           = ":8000"
)

// StateRequest represents a request to change printer state.
type StateRequest struct {
	State string `json:"state" binding:"required,oneof=ON OFF"`
}

// StateResponse represents the response with printer state.
type StateResponse struct {
	State string `json:"state"`
}

// Config holds application configuration.
type Config struct {
	MQTTHost      string
	MQTTUser      string
	MQTTPass      string
	SSHHost       string
	SSHUser       string
	SSHPass       string
	MoonrakerURL  string
	ThresholdTemp int
}

// loadConfig loads the application configuration from environment variables.
func loadConfig() (*Config, error) {
	_ = godotenv.Load(".env")
	_ = godotenv.Load("/root/power-api/.env")

	return &Config{
		MQTTHost:      getenvString("mqtt_host", ""),
		MQTTUser:      getenvString("mqtt_user", ""),
		MQTTPass:      getenvString("mqtt_pass", ""),
		SSHHost:       getenvString("ssh_host", ""),
		SSHUser:       getenvString("ssh_user", ""),
		SSHPass:       getenvString("ssh_pass", ""),
		MoonrakerURL:  getenvString("moonraker_url", ""),
		ThresholdTemp: getenvInt("threshold_temp", defaultThresholdTemp),
	}, nil
}

// getenvString retrieves a string environment variable with a default value.
func getenvString(key, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return defaultVal
}

// getenvInt retrieves an integer environment variable with a default value.
func getenvInt(key string, defaultVal int) int {
	val, ok := os.LookupEnv(key)
	if !ok {
		return defaultVal
	}
	parsed, err := strconv.Atoi(val)
	if err != nil {
		return defaultVal
	}
	return parsed
}

// printerStatusResponse represents the printer status from Moonraker API.
type printerStatusResponse struct {
	Result struct {
		Status struct {
			PrintStats struct {
				State string `json:"state"`
			} `json:"print_stats"`
		} `json:"status"`
	} `json:"result"`
}

// temperatureResponse represents the temperature data from Moonraker API.
type temperatureResponse struct {
	Result struct {
		Extruder struct {
			Temperatures []float64 `json:"temperatures"`
		} `json:"extruder"`
	} `json:"result"`
}

// mqttStateMessage represents the state message structure from MQTT.
type mqttStateMessage struct {
	State string `json:"state"`
}

// isPrinterFinished checks if the printer has finished printing.
func isPrinterFinished(ctx context.Context, baseURL string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/printer/objects/query?print_stats", baseURL), nil)
	if err != nil {
		return false, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	var result printerStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, fmt.Errorf("failed to decode printer status: %w", err)
	}

	state := result.Result.Status.PrintStats.State
	return state == "standby" || state == "complete", nil
}

// getCurrentExtruderTemperature fetches and calculates the average extruder temperature.
func getCurrentExtruderTemperature(ctx context.Context, baseURL string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/server/temperature_store", baseURL), nil)
	if err != nil {
		return 0, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var result temperatureResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("failed to decode temperature data: %w", err)
	}

	temps := result.Result.Extruder.Temperatures
	if len(temps) <= 10 {
		return 0, fmt.Errorf("insufficient temperature data: got %d samples", len(temps))
	}

	// Keep only the last 10 samples
	const maxSamples = 10
	if len(temps) > maxSamples {
		temps = temps[len(temps)-maxSamples:]
	}

	// Filter out invalid readings
	const minTemp = 1
	const maxTemp = 400
	var filtered []float64
	for _, v := range temps {
		if v >= minTemp && v <= maxTemp {
			filtered = append(filtered, v)
		}
	}

	const minValidReadings = 5
	if len(filtered) <= minValidReadings {
		return 0, fmt.Errorf("not enough valid temperature readings after filtering: %d", len(filtered))
	}

	// Calculate average
	sum := 0.0
	for _, v := range filtered {
		sum += v
	}
	avg := sum / float64(len(filtered))
	return int(avg), nil
}

// sendSSHCommand executes a command on a remote SSH host.
func sendSSHCommand(ctx context.Context, host, user, pass, command string) error {
	config := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.Password(pass)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:22", host), config)
	if err != nil {
		return fmt.Errorf("failed to connect via SSH: %w", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create SSH session: %w", err)
	}
	defer session.Close()

	// Execute command with context support via goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- session.Run(command)
	}()

	select {
	case err := <-errChan:
		if err != nil {
			return fmt.Errorf("failed to execute command: %w", err)
		}
	case <-ctx.Done():
		session.Close()
		return fmt.Errorf("command execution cancelled: %w", ctx.Err())
	}
	return nil
}

// isHostReachable checks if a host is reachable via ping.
func isHostReachable(ctx context.Context, host string) bool {
	cmd := exec.CommandContext(ctx, "ping", "-c", "1", host)
	return cmd.Run() == nil
}

// getMQTTState retrieves the current state from an MQTT topic with a timeout.
func getMQTTState(ctx context.Context, client mqtt.Client, topic string) (string, error) {
	messageChan := make(chan string, 1)
	errChan := make(chan error, 1)

	callback := func(_ mqtt.Client, msg mqtt.Message) {
		var response mqttStateMessage
		if err := json.Unmarshal(msg.Payload(), &response); err != nil {
			errChan <- fmt.Errorf("failed to parse MQTT message: %w", err)
			return
		}
		messageChan <- response.State
	}

	token := client.Subscribe(topic, 0, callback)
	if !token.WaitTimeout(mqttCommandTimeout) || token.Error() != nil {
		return "", fmt.Errorf("failed to subscribe to topic %s: %v", topic, token.Error())
	}
	defer func() {
		token := client.Unsubscribe(topic)
		if !token.WaitTimeout(mqttCommandTimeout) || token.Error() != nil {
			log.Printf("warning: failed to unsubscribe from topic %s: %v", topic, token.Error())
		}
	}()

	select {
	case state := <-messageChan:
		return state, nil
	case err := <-errChan:
		return "", err
	case <-ctx.Done():
		return "", fmt.Errorf("context cancelled: %w", ctx.Err())
	}
}

// newMQTTClient creates and connects to an MQTT broker.
func newMQTTClient(host, user, pass string) (mqtt.Client, error) {
	opts := mqtt.NewClientOptions().
		AddBroker(fmt.Sprintf("tcp://%s:1883", host)).
		SetUsername(user).
		SetPassword(pass).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(5 * time.Second).
		SetMaxReconnectInterval(1 * time.Minute).
		SetKeepAlive(30 * time.Second).
		SetConnectionLostHandler(func(_ mqtt.Client, err error) {
			log.Printf("MQTT connection lost: %v", err)
		}).
		SetOnConnectHandler(func(_ mqtt.Client) {
			log.Println("Connected to MQTT broker")
		}).
		SetReconnectingHandler(func(_ mqtt.Client, _ *mqtt.ClientOptions) {
			log.Println("Attempting to reconnect to MQTT broker...")
		})

	client := mqtt.NewClient(opts)
	token := client.Connect()
	if !token.WaitTimeout(mqttConnectTimeout) {
		return nil, fmt.Errorf("MQTT connection timeout")
	}
	if token.Error() != nil {
		return nil, fmt.Errorf("failed to connect to MQTT broker: %w", token.Error())
	}

	return client, nil
}

func main() {
	config, err := loadConfig()
	if err != nil {
		log.Fatalf("failed to load configuration: %v", err)
	}

	mqttClient, err := newMQTTClient(config.MQTTHost, config.MQTTUser, config.MQTTPass)
	if err != nil {
		log.Fatalf("failed to connect to MQTT: %v", err)
	}
	defer mqttClient.Disconnect(250)

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()

	// GET printer state
	router.GET("/api/3d-printer", handleGetPrinterState(mqttClient, config))

	// POST to control printer
	router.POST("/api/3d-printer", handlePostPrinterControl(mqttClient, config))

	log.Println("Starting server on :8000")
	if err := router.Run(serverPort); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func handleGetPrinterState(client mqtt.Client, cfg *Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), mqttCommandTimeout)
		defer cancel()

		state, err := getMQTTState(ctx, client, "zigbee2mqtt/R")
		if err != nil {
			log.Printf("failed to get printer state: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve printer state"})
			return
		}
		c.JSON(http.StatusOK, StateResponse{State: state})
	}
}

func handlePostPrinterControl(client mqtt.Client, cfg *Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req StateRequest
		if err := c.BindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		ctx := c.Request.Context()

		switch req.State {
		case "ON":
			if err := publishMQTTState(client, "zigbee2mqtt/R/set", "ON"); err != nil {
				log.Printf("failed to publish ON state: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to turn on printer"})
				return
			}
			c.JSON(http.StatusOK, StateResponse{State: "ON"})

		case "OFF":
			if err := shutdownPrinter(ctx, client, cfg); err != nil {
				log.Printf("shutdown error: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, StateResponse{State: "OFF"})

		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid state"})
		}
	}
}

func shutdownPrinter(ctx context.Context, client mqtt.Client, cfg *Config) error {
	// Wait for printer to finish and cool down
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		finished, err := isPrinterFinished(ctx, cfg.MoonrakerURL)
		if err != nil {
			log.Printf("error checking printer status: %v", err)
			continue
		}

		temp, err := getCurrentExtruderTemperature(ctx, cfg.MoonrakerURL)
		if err != nil {
			log.Printf("error getting temperature: %v", err)
			continue
		}

		if finished && temp < cfg.ThresholdTemp {
			break
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}
	}

	// Shutdown the remote host
	shutdownCtx, cancel := context.WithTimeout(ctx, sshCommandTimeout)
	defer cancel()
	if err := sendSSHCommand(shutdownCtx, cfg.SSHHost, cfg.SSHUser, cfg.SSHPass, "/sbin/shutdown 0"); err != nil {
		log.Printf("failed to send shutdown command: %v", err)
	}

	// Wait for host to go offline
	pollCtx, cancel := context.WithTimeout(ctx, hostPollTimeout)
	defer cancel()
	for isHostReachable(pollCtx, cfg.SSHHost) {
		select {
		case <-pollCtx.Done():
			break
		case <-time.After(pollInterval):
		}
	}

	// Turn off the relay
	if err := publishMQTTState(client, "zigbee2mqtt/R/set", "OFF"); err != nil {
		return fmt.Errorf("failed to publish OFF state: %w", err)
	}

	return nil
}

func publishMQTTState(client mqtt.Client, topic, state string) error {
	payload := fmt.Sprintf(`{"state": "%s"}`, state)
	token := client.Publish(topic, 0, false, payload)
	if !token.WaitTimeout(5 * time.Second) || token.Error() != nil {
		return fmt.Errorf("failed to publish to topic %s: %v", topic, token.Error())
	}
	return nil
}

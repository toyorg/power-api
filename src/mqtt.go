package powerapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

func onMQTTConnectionLost(_ mqtt.Client, err error) {
	log.Printf("MQTT connection lost: %v", err)
}

func onMQTTConnect(_ mqtt.Client) {
	log.Println("Connected to MQTT broker")
}

func onMQTTReconnecting(_ mqtt.Client, _ *mqtt.ClientOptions) {
	log.Println("Attempting to reconnect to MQTT broker...")
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
	return newMQTTClientWithFactory(host, user, pass, mqtt.NewClient)
}

func newMQTTClientWithFactory(host, user, pass string, newClientFn func(*mqtt.ClientOptions) mqtt.Client) (mqtt.Client, error) {
	opts := mqtt.NewClientOptions().
		AddBroker(fmt.Sprintf("tcp://%s:1883", host)).
		SetUsername(user).
		SetPassword(pass).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(5 * time.Second).
		SetMaxReconnectInterval(1 * time.Minute).
		SetKeepAlive(30 * time.Second).
		SetConnectionLostHandler(onMQTTConnectionLost).
		SetOnConnectHandler(onMQTTConnect).
		SetReconnectingHandler(onMQTTReconnecting)

	client := newClientFn(opts)

	// Retry connecting indefinitely until successful
	for {
		token := client.Connect()
		if token.WaitTimeout(mqttConnectTimeout) && token.Error() == nil {
			break // Connected successfully
		}
		log.Printf("Failed to connect to MQTT broker, retrying in 5 seconds...")
		time.Sleep(5 * time.Second)
	}

	return client, nil
}

func publishMQTTState(client mqtt.Client, topic, state string) error {
	payload := fmt.Sprintf(`{"state": "%s"}`, state)
	token := client.Publish(topic, 0, false, payload)
	if !token.WaitTimeout(5*time.Second) || token.Error() != nil {
		return fmt.Errorf("failed to publish to topic %s: %v", topic, token.Error())
	}
	return nil
}

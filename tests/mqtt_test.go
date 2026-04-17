package tests

import (
	"errors"
	"strings"
	"testing"

	powerapi "power-api/src"
)

func TestPublishMQTTState(t *testing.T) {
	client := &fakeMQTTClient{publishToken: &fakeToken{waitResult: true}}

	err := powerapi.PublishMQTTState(client, "zigbee2mqtt/R/set", "ON")
	if err != nil {
		t.Fatalf("publishMQTTState returned error: %v", err)
	}

	if client.publishedTopic != "zigbee2mqtt/R/set" {
		t.Fatalf("unexpected topic: %s", client.publishedTopic)
	}
	if client.publishedQos != 0 || client.publishedRet {
		t.Fatalf("unexpected QoS/retained: qos=%d retained=%v", client.publishedQos, client.publishedRet)
	}
	payload, ok := client.payload.(string)
	if !ok {
		t.Fatalf("expected payload as string, got %T", client.payload)
	}
	if payload != `{"state": "ON"}` {
		t.Fatalf("unexpected payload: %s", payload)
	}
}

func TestPublishMQTTState_WaitTimeoutError(t *testing.T) {
	client := &fakeMQTTClient{publishToken: &fakeToken{waitResult: false}}

	err := powerapi.PublishMQTTState(client, "zigbee2mqtt/R/set", "OFF")
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to publish") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPublishMQTTState_TokenError(t *testing.T) {
	client := &fakeMQTTClient{publishToken: &fakeToken{waitResult: true, err: errors.New("broker failed")}}

	err := powerapi.PublishMQTTState(client, "zigbee2mqtt/R/set", "OFF")
	if err == nil {
		t.Fatal("expected token error, got nil")
	}
	if !strings.Contains(err.Error(), "broker failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

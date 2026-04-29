package powerapi

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestPublishMQTTState(t *testing.T) {
	client := &fakeMQTTClient{publishToken: &fakeToken{waitResult: true}}

	err := publishMQTTState(client, topicPrinterSet, stateON)
	if err != nil {
		t.Fatalf("publishMQTTState returned error: %v", err)
	}

	if client.publishedTopic != topicPrinterSet {
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

	err := publishMQTTState(client, topicPrinterSet, stateOFF)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to publish") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPublishMQTTState_TokenError(t *testing.T) {
	client := &fakeMQTTClient{publishToken: &fakeToken{waitResult: true, err: errors.New("broker failed")}}

	err := publishMQTTState(client, topicPrinterSet, stateOFF)
	if err == nil {
		t.Fatal("expected token error, got nil")
	}
	if !strings.Contains(err.Error(), "broker failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetMQTTState_ContextCanceled(t *testing.T) {
	client := &fakeMQTTClient{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := getMQTTState(ctx, client, topicPrinterState)
	if err == nil {
		t.Fatal("expected context cancellation error, got nil")
	}
	if !strings.Contains(err.Error(), "context cancelled") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetMQTTState_SubscribeWaitTimeout(t *testing.T) {
	client := &fakeMQTTClient{subscribeToken: &fakeToken{waitResult: false}}

	_, err := getMQTTState(context.Background(), client, topicPrinterState)
	if err == nil {
		t.Fatal("expected subscribe timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to subscribe") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetMQTTState_SubscribeTokenError(t *testing.T) {
	client := &fakeMQTTClient{subscribeToken: &fakeToken{waitResult: true, err: errors.New("not authorized")}}

	_, err := getMQTTState(context.Background(), client, topicPrinterState)
	if err == nil {
		t.Fatal("expected subscribe token error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to subscribe") {
		t.Fatalf("unexpected error: %v", err)
	}
}

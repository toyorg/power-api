package tests

import (
	"context"
	"errors"
	"strings"
	"testing"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/gin-gonic/gin"
	powerapi "power-api/src"
)

func TestGetMQTTState_ContextCanceled(t *testing.T) {
	client := &fakeMQTTClient{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := powerapi.GetMQTTState(ctx, client, "zigbee2mqtt/R")
	if err == nil {
		t.Fatal("expected context cancellation error, got nil")
	}
	if !strings.Contains(err.Error(), "context cancelled") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetMQTTState_SubscribeWaitTimeout(t *testing.T) {
	client := &fakeMQTTClient{subscribeToken: &fakeToken{waitResult: false}}

	_, err := powerapi.GetMQTTState(context.Background(), client, "zigbee2mqtt/R")
	if err == nil {
		t.Fatal("expected subscribe timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to subscribe") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetMQTTState_SubscribeTokenError(t *testing.T) {
	client := &fakeMQTTClient{subscribeToken: &fakeToken{waitResult: true, err: errors.New("not authorized")}}

	_, err := powerapi.GetMQTTState(context.Background(), client, "zigbee2mqtt/R")
	if err == nil {
		t.Fatal("expected subscribe token error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to subscribe") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDefaultShutdownDeps_ExposesUsableFunctions(t *testing.T) {
	client := &fakeMQTTClient{publishToken: &fakeToken{waitResult: true}}
	deps := powerapi.DefaultShutdownDeps(client)

	if deps.PollInterval <= 0 {
		t.Fatalf("expected positive poll interval, got %v", deps.PollInterval)
	}
	if deps.Sleep == nil || deps.IsPrinterFinished == nil || deps.GetCurrentExtruderTemp == nil || deps.PublishMQTTState == nil {
		t.Fatal("expected all default dependency functions to be set")
	}

	err := deps.PublishMQTTState("zigbee2mqtt/R/set", "ON")
	if err != nil {
		t.Fatalf("default publish function returned error: %v", err)
	}

	client.publishToken = &fakeToken{waitResult: true, err: errors.New("boom")}
	err = deps.PublishMQTTState("zigbee2mqtt/R/set", "ON")
	if err == nil {
		t.Fatal("expected publish error, got nil")
	}
}

func TestMQTTCallbackHooks_AreCallable(t *testing.T) {
	client := &fakeMQTTClient{}
	powerapi.OnMQTTConnect(client)
	powerapi.OnMQTTConnectionLost(client, errors.New("lost"))
	powerapi.OnMQTTReconnecting(client, &mqtt.ClientOptions{})
}

func TestNewPostPrinterControlHandlerWithShutdown_NilFnFallsBack(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := &fakeMQTTClient{publishToken: &fakeToken{waitResult: true}}

	h := powerapi.NewPostPrinterControlHandlerWithShutdown(client, &powerapi.Config{}, nil)
	if h == nil {
		t.Fatal("expected handler, got nil")
	}
}

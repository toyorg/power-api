package tests

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	powerapi "power-api/src"
)

func TestAPI_GetPrinterState_OK(t *testing.T) {
	gin.SetMode(gin.TestMode)

	client := &fakeMQTTClient{
		subscribeBody: `{"state":"ON"}`,
	}
	router := gin.New()
	router.GET("/api/3d-printer", powerapi.NewGetPrinterStateHandler(client, &powerapi.Config{}))

	req := httptest.NewRequest(http.MethodGet, "/api/3d-printer", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != `{"state":"ON"}` {
		t.Fatalf("unexpected response body: %s", got)
	}
	if client.subscribedTo != "zigbee2mqtt/R" {
		t.Fatalf("expected subscription to zigbee2mqtt/R, got %s", client.subscribedTo)
	}
}

func TestAPI_GetPrinterState_InvalidMQTTPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)

	client := &fakeMQTTClient{subscribeBody: `{"state":`}
	router := gin.New()
	router.GET("/api/3d-printer", powerapi.NewGetPrinterStateHandler(client, &powerapi.Config{}))

	req := httptest.NewRequest(http.MethodGet, "/api/3d-printer", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAPI_GetPrinterState_SubscribeError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	client := &fakeMQTTClient{subscribeToken: &fakeToken{waitResult: true, err: errors.New("subscribe failed")}}
	router := gin.New()
	router.GET("/api/3d-printer", powerapi.NewGetPrinterStateHandler(client, &powerapi.Config{}))

	req := httptest.NewRequest(http.MethodGet, "/api/3d-printer", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAPI_PostPrinterState_ON_OK(t *testing.T) {
	gin.SetMode(gin.TestMode)

	client := &fakeMQTTClient{publishToken: &fakeToken{waitResult: true}}
	router := gin.New()
	router.POST("/api/3d-printer", powerapi.NewPostPrinterControlHandler(client, &powerapi.Config{}))

	req := httptest.NewRequest(http.MethodPost, "/api/3d-printer", bytes.NewBufferString(`{"state":"ON"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != `{"state":"ON"}` {
		t.Fatalf("unexpected response body: %s", got)
	}
	if client.publishedTopic != "zigbee2mqtt/R/set" {
		t.Fatalf("expected publish topic zigbee2mqtt/R/set, got %s", client.publishedTopic)
	}
	if payload, _ := client.payload.(string); payload != `{"state": "ON"}` {
		t.Fatalf("unexpected published payload: %v", client.payload)
	}
}

func TestAPI_PostPrinterState_ON_PublishError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	client := &fakeMQTTClient{publishToken: &fakeToken{waitResult: true, err: errors.New("publish failed")}}
	router := gin.New()
	router.POST("/api/3d-printer", powerapi.NewPostPrinterControlHandler(client, &powerapi.Config{}))

	req := httptest.NewRequest(http.MethodPost, "/api/3d-printer", bytes.NewBufferString(`{"state":"ON"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAPI_PostPrinterState_InvalidRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	client := &fakeMQTTClient{}
	router := gin.New()
	router.POST("/api/3d-printer", powerapi.NewPostPrinterControlHandler(client, &powerapi.Config{}))

	req := httptest.NewRequest(http.MethodPost, "/api/3d-printer", bytes.NewBufferString(`{"state":"INVALID"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		body, _ := io.ReadAll(rec.Body)
		t.Fatalf("expected status 400, got %d body=%s", rec.Code, string(body))
	}
}

func TestAPI_PostPrinterState_OFF_OK(t *testing.T) {
	gin.SetMode(gin.TestMode)

	client := &fakeMQTTClient{}
	router := gin.New()
	router.POST("/api/3d-printer", powerapi.NewPostPrinterControlHandlerWithShutdown(client, &powerapi.Config{}, func(_ *powerapi.Config, _ powerapi.ShutdownDeps) error {
		return nil
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/3d-printer", bytes.NewBufferString(`{"state":"OFF"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if got := strings.TrimSpace(rec.Body.String()); got != `{"state":"OFF"}` {
		t.Fatalf("unexpected response body: %s", got)
	}
}

func TestAPI_PostPrinterState_OFF_ShutdownError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	client := &fakeMQTTClient{}
	router := gin.New()
	router.POST("/api/3d-printer", powerapi.NewPostPrinterControlHandlerWithShutdown(client, &powerapi.Config{}, func(_ *powerapi.Config, _ powerapi.ShutdownDeps) error {
		return errors.New("shutdown failed")
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/3d-printer", bytes.NewBufferString(`{"state":"OFF"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d body=%s", rec.Code, rec.Body.String())
	}
}

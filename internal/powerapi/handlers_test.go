package powerapi

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestAPI_GetPrinterState_OK(t *testing.T) {
	gin.SetMode(gin.TestMode)

	client := &fakeMQTTClient{
		subscribeBody: `{"state":"ON"}`,
	}
	router := gin.New()
	router.GET("/api/3d-printer", handleGetPrinterState(client, &Config{}))

	req := httptest.NewRequest(http.MethodGet, "/api/3d-printer", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != `{"state":"ON"}` {
		t.Fatalf("unexpected response body: %s", got)
	}
	if client.subscribedTo != topicPrinterState {
		t.Fatalf("expected subscription to %s, got %s", topicPrinterState, client.subscribedTo)
	}
}

func TestAPI_GetPrinterState_InvalidMQTTPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)

	client := &fakeMQTTClient{subscribeBody: `{"state":`}
	router := gin.New()
	router.GET("/api/3d-printer", handleGetPrinterState(client, &Config{}))

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
	router.GET("/api/3d-printer", handleGetPrinterState(client, &Config{}))

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
	router.POST("/api/3d-printer", handlePostPrinterControl(client, &Config{}))

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
	if client.publishedTopic != topicPrinterSet {
		t.Fatalf("expected publish topic %s, got %s", topicPrinterSet, client.publishedTopic)
	}
	if payload, _ := client.payload.(string); payload != `{"state": "ON"}` {
		t.Fatalf("unexpected published payload: %v", client.payload)
	}
}

func TestAPI_PostPrinterState_ON_PublishError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	client := &fakeMQTTClient{publishToken: &fakeToken{waitResult: true, err: errors.New("publish failed")}}
	router := gin.New()
	router.POST("/api/3d-printer", handlePostPrinterControl(client, &Config{}))

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
	router.POST("/api/3d-printer", handlePostPrinterControl(client, &Config{}))

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
	router.POST("/api/3d-printer", handlePostPrinterControlWithShutdown(client, &Config{}, func(_ *Config, _ shutdownDeps) error {
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
	router.POST("/api/3d-printer", handlePostPrinterControlWithShutdown(client, &Config{}, func(_ *Config, _ shutdownDeps) error {
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

func TestAPI_PostPrinterState_OFF_OnlyOnePendingShutdown(t *testing.T) {
	gin.SetMode(gin.TestMode)

	client := &fakeMQTTClient{}
	router := gin.New()

	started := make(chan struct{})
	finish := make(chan struct{})
	var startedOnce sync.Once
	var shutdownCalls atomic.Int32

	router.POST("/api/3d-printer", handlePostPrinterControlWithShutdown(client, &Config{}, func(_ *Config, _ shutdownDeps) error {
		shutdownCalls.Add(1)
		startedOnce.Do(func() { close(started) })
		<-finish
		return nil
	}))

	firstReq := httptest.NewRequest(http.MethodPost, "/api/3d-printer", bytes.NewBufferString(`{"state":"OFF"}`))
	firstReq.Header.Set("Content-Type", "application/json")
	firstRec := httptest.NewRecorder()

	firstDone := make(chan struct{})
	go func() {
		router.ServeHTTP(firstRec, firstReq)
		close(firstDone)
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("first shutdown did not start in time")
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/api/3d-printer", bytes.NewBufferString(`{"state":"ON"}`))
	secondReq.Header.Set("Content-Type", "application/json")
	secondRec := httptest.NewRecorder()
	router.ServeHTTP(secondRec, secondReq)

	if secondRec.Code != http.StatusOK {
		t.Fatalf("expected second request status 200, got %d body=%s", secondRec.Code, secondRec.Body.String())
	}
	if got := strings.TrimSpace(secondRec.Body.String()); got != `{"state":"ON"}` {
		t.Fatalf("unexpected second response body: %s", got)
	}

	if got := shutdownCalls.Load(); got != 1 {
		t.Fatalf("expected one shutdown call while pending, got %d", shutdownCalls.Load())
	}

	close(finish)

	select {
	case <-firstDone:
	case <-time.After(2 * time.Second):
		t.Fatal("first shutdown request did not complete in time")
	}

	if firstRec.Code != http.StatusOK {
		t.Fatalf("expected first request status 200, got %d body=%s", firstRec.Code, firstRec.Body.String())
	}
	if got := strings.TrimSpace(firstRec.Body.String()); got != `{"state":"OFF"}` {
		t.Fatalf("unexpected first response body: %s", got)
	}

	if got := shutdownCalls.Load(); got != 1 {
		t.Fatalf("expected total shutdown call count to stay at 1, got %d", shutdownCalls.Load())
	}
}

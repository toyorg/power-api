package main

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/gin-gonic/gin"
)

func TestShutdownPrinter_ResponsesReturnedNow(t *testing.T) {
	cfg := &Config{
		MoonrakerURL:  "http://moonraker.local",
		SSHHost:       "printer-host",
		SSHUser:       "root",
		SSHPass:       "secret",
		ThresholdTemp: defaultThresholdTemp,
	}

	var (
		finishedCalls int
		tempCalls     int
		sshCalls      int
		publishCalls  int
		sleepCalls    int
	)

	deps := shutdownDeps{
		isPrinterFinished: func(baseURL string) (bool, error) {
			finishedCalls++
			if baseURL != cfg.MoonrakerURL {
				t.Fatalf("unexpected moonraker URL: %s", baseURL)
			}
			return true, nil
		},
		getCurrentExtruderTemp: func(baseURL string) (int, error) {
			tempCalls++
			if baseURL != cfg.MoonrakerURL {
				t.Fatalf("unexpected moonraker URL: %s", baseURL)
			}
			return cfg.ThresholdTemp - 1, nil
		},
		sendSSHCommand: func(host, user, pass, command string) error {
			sshCalls++
			if host != cfg.SSHHost || user != cfg.SSHUser || pass != cfg.SSHPass {
				t.Fatalf("unexpected ssh args: host=%s user=%s pass=%s", host, user, pass)
			}
			if command != "/sbin/shutdown 0" {
				t.Fatalf("unexpected ssh command: %s", command)
			}
			return nil
		},
		isHostReachable: func(host string) bool {
			if host != cfg.SSHHost {
				t.Fatalf("unexpected host in reachability check: %s", host)
			}
			return false
		},
		publishMQTTState: func(topic, state string) error {
			publishCalls++
			if topic != "zigbee2mqtt/R/set" || state != "OFF" {
				t.Fatalf("unexpected mqtt publish args: topic=%s state=%s", topic, state)
			}
			return nil
		},
		sleep: func(time.Duration) {
			sleepCalls++
		},
		pollInterval: pollInterval,
	}

	if err := shutdownPrinter(cfg, deps); err != nil {
		t.Fatalf("shutdownPrinter returned error: %v", err)
	}

	if finishedCalls != 1 {
		t.Fatalf("expected 1 status check, got %d", finishedCalls)
	}
	if tempCalls != 1 {
		t.Fatalf("expected 1 temperature check, got %d", tempCalls)
	}
	if sshCalls != 1 {
		t.Fatalf("expected 1 ssh command, got %d", sshCalls)
	}
	if publishCalls != 1 {
		t.Fatalf("expected 1 mqtt publish, got %d", publishCalls)
	}
	if sleepCalls != 0 {
		t.Fatalf("expected no sleeping for immediate response path, got %d sleeps", sleepCalls)
	}
}

func TestShutdownPrinter_ResponsesReturnedAfter30Seconds(t *testing.T) {
	cfg := &Config{
		MoonrakerURL:  "http://moonraker.local",
		SSHHost:       "printer-host",
		SSHUser:       "root",
		SSHPass:       "secret",
		ThresholdTemp: defaultThresholdTemp,
	}

	var (
		statusCalls  int
		tempCalls    int
		sshCalls     int
		publishCalls int
		sleepCalls   int
		elapsed      time.Duration
	)

	deps := shutdownDeps{
		isPrinterFinished: func(string) (bool, error) {
			statusCalls++
			if elapsed < 30*time.Second {
				return false, errors.New("moonraker status not available yet")
			}
			return true, nil
		},
		getCurrentExtruderTemp: func(string) (int, error) {
			tempCalls++
			if elapsed < 30*time.Second {
				return 0, errors.New("temperature endpoint not available yet")
			}
			return cfg.ThresholdTemp - 1, nil
		},
		sendSSHCommand: func(host, user, pass, command string) error {
			sshCalls++
			return nil
		},
		isHostReachable: func(string) bool {
			return false
		},
		publishMQTTState: func(topic, state string) error {
			publishCalls++
			return nil
		},
		sleep: func(d time.Duration) {
			sleepCalls++
			elapsed += d
		},
		pollInterval: pollInterval,
	}

	if err := shutdownPrinter(cfg, deps); err != nil {
		t.Fatalf("shutdownPrinter returned error: %v", err)
	}

	if elapsed != 30*time.Second {
		t.Fatalf("expected virtual elapsed time to be 30s, got %v", elapsed)
	}
	if sleepCalls != 6 {
		t.Fatalf("expected 6 sleep calls (6 * 5s = 30s), got %d", sleepCalls)
	}
	if statusCalls != 7 {
		t.Fatalf("expected 7 status checks (including final successful one), got %d", statusCalls)
	}
	if tempCalls != 1 {
		t.Fatalf("expected 1 temperature check after status became available, got %d", tempCalls)
	}
	if sshCalls != 1 {
		t.Fatalf("expected 1 ssh command, got %d", sshCalls)
	}
	if publishCalls != 1 {
		t.Fatalf("expected 1 mqtt publish, got %d", publishCalls)
	}
}

type fakeToken struct {
	waitResult bool
	err        error
	done       chan struct{}
}

func (t *fakeToken) Wait() bool {
	return t.waitResult
}

func (t *fakeToken) WaitTimeout(_ time.Duration) bool {
	return t.waitResult
}

func (t *fakeToken) Done() <-chan struct{} {
	if t.done == nil {
		t.done = make(chan struct{})
		close(t.done)
	}
	return t.done
}

func (t *fakeToken) Error() error {
	return t.err
}

type fakeMQTTClient struct {
	publishToken   mqtt.Token
	subscribeToken mqtt.Token
	publishedTopic string
	publishedQos   byte
	publishedRet   bool
	payload        interface{}
	subscribedTo   string
	subscribeBody  string
}

func (c *fakeMQTTClient) IsConnected() bool { return true }

func (c *fakeMQTTClient) IsConnectionOpen() bool { return true }

func (c *fakeMQTTClient) Connect() mqtt.Token { return &fakeToken{waitResult: true} }

func (c *fakeMQTTClient) Disconnect(_ uint) {}

func (c *fakeMQTTClient) Publish(topic string, qos byte, retained bool, payload interface{}) mqtt.Token {
	c.publishedTopic = topic
	c.publishedQos = qos
	c.publishedRet = retained
	c.payload = payload
	if c.publishToken == nil {
		return &fakeToken{waitResult: true}
	}
	return c.publishToken
}

func (c *fakeMQTTClient) Subscribe(topic string, _ byte, callback mqtt.MessageHandler) mqtt.Token {
	c.subscribedTo = topic
	if callback != nil && c.subscribeBody != "" {
		callback(c, &fakeMessage{topic: topic, payload: []byte(c.subscribeBody)})
	}
	if c.subscribeToken == nil {
		return &fakeToken{waitResult: true}
	}
	return c.subscribeToken
}

func (c *fakeMQTTClient) SubscribeMultiple(_ map[string]byte, _ mqtt.MessageHandler) mqtt.Token {
	return &fakeToken{waitResult: true}
}

func (c *fakeMQTTClient) Unsubscribe(_ ...string) mqtt.Token { return &fakeToken{waitResult: true} }

func (c *fakeMQTTClient) AddRoute(_ string, _ mqtt.MessageHandler) {}

func (c *fakeMQTTClient) OptionsReader() mqtt.ClientOptionsReader { return mqtt.ClientOptionsReader{} }

type fakeMessage struct {
	topic   string
	payload []byte
}

func (m *fakeMessage) Duplicate() bool  { return false }
func (m *fakeMessage) Qos() byte        { return 0 }
func (m *fakeMessage) Retained() bool   { return false }
func (m *fakeMessage) Topic() string    { return m.topic }
func (m *fakeMessage) MessageID() uint16 { return 0 }
func (m *fakeMessage) Payload() []byte  { return m.payload }
func (m *fakeMessage) Ack()             {}

func TestGetenvStringAndGetenvInt(t *testing.T) {
	t.Setenv("power_api_test_str", "abc")
	t.Setenv("power_api_test_int", "123")
	t.Setenv("power_api_test_bad_int", "bad")

	if got := getenvString("power_api_test_str", "fallback"); got != "abc" {
		t.Fatalf("expected getenvString to return env value, got %q", got)
	}
	if got := getenvString("power_api_test_missing_str", "fallback"); got != "fallback" {
		t.Fatalf("expected getenvString fallback, got %q", got)
	}

	if got := getenvInt("power_api_test_int", 7); got != 123 {
		t.Fatalf("expected getenvInt parsed value 123, got %d", got)
	}
	if got := getenvInt("power_api_test_bad_int", 7); got != 7 {
		t.Fatalf("expected getenvInt fallback for bad value, got %d", got)
	}
	if got := getenvInt("power_api_test_missing_int", 7); got != 7 {
		t.Fatalf("expected getenvInt fallback for missing value, got %d", got)
	}
}

func TestLoadConfig(t *testing.T) {
	t.Setenv("mqtt_host", "broker.local")
	t.Setenv("mqtt_user", "user")
	t.Setenv("mqtt_pass", "pass")
	t.Setenv("ssh_host", "ssh.local")
	t.Setenv("ssh_user", "root")
	t.Setenv("ssh_pass", "secret")
	t.Setenv("moonraker_url", "http://moonraker.local")
	t.Setenv("threshold_temp", "55")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig returned error: %v", err)
	}

	if cfg.MQTTHost != "broker.local" || cfg.MQTTUser != "user" || cfg.MQTTPass != "pass" {
		t.Fatalf("unexpected MQTT config: %+v", cfg)
	}
	if cfg.SSHHost != "ssh.local" || cfg.SSHUser != "root" || cfg.SSHPass != "secret" {
		t.Fatalf("unexpected SSH config: %+v", cfg)
	}
	if cfg.MoonrakerURL != "http://moonraker.local" {
		t.Fatalf("unexpected moonraker URL: %s", cfg.MoonrakerURL)
	}
	if cfg.ThresholdTemp != 55 {
		t.Fatalf("unexpected threshold temperature: %d", cfg.ThresholdTemp)
	}
}

func TestIsPrinterFinished(t *testing.T) {
	tests := []struct {
		name     string
		state    string
		expected bool
	}{
		{name: "standby is finished", state: "standby", expected: true},
		{name: "complete is finished", state: "complete", expected: true},
		{name: "printing is not finished", state: "printing", expected: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/printer/objects/query" {
					t.Fatalf("unexpected path: %s", r.URL.Path)
				}
				if r.URL.RawQuery != "print_stats" {
					t.Fatalf("unexpected query: %s", r.URL.RawQuery)
				}
				_, _ = w.Write([]byte(`{"result":{"status":{"print_stats":{"state":"` + tc.state + `"}}}}`))
			}))
			defer server.Close()

			finished, err := isPrinterFinished(server.URL)
			if err != nil {
				t.Fatalf("isPrinterFinished returned error: %v", err)
			}
			if finished != tc.expected {
				t.Fatalf("expected %v, got %v", tc.expected, finished)
			}
		})
	}
}

func TestIsPrinterFinished_DecodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("{"))
	}))
	defer server.Close()

	_, err := isPrinterFinished(server.URL)
	if err == nil {
		t.Fatal("expected decode error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to decode printer status") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetCurrentExtruderTemperature(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/server/temperature_store" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"result":{"extruder":{"temperatures":[0,500,10,20,30,40,50,60,70,80,90,100]}}}`))
	}))
	defer server.Close()

	temp, err := getCurrentExtruderTemperature(server.URL)
	if err != nil {
		t.Fatalf("getCurrentExtruderTemperature returned error: %v", err)
	}

	if temp != 55 {
		t.Fatalf("expected average temp 55, got %d", temp)
	}
}

func TestGetCurrentExtruderTemperature_InsufficientSamples(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"result":{"extruder":{"temperatures":[1,2,3,4,5,6,7,8,9,10]}}}`))
	}))
	defer server.Close()

	_, err := getCurrentExtruderTemperature(server.URL)
	if err == nil {
		t.Fatal("expected insufficient samples error, got nil")
	}
	if !strings.Contains(err.Error(), "insufficient temperature data") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetCurrentExtruderTemperature_NotEnoughValidReadings(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"result":{"extruder":{"temperatures":[0,0,0,0,0,0,0,0,0,2,3]}}}`))
	}))
	defer server.Close()

	_, err := getCurrentExtruderTemperature(server.URL)
	if err == nil {
		t.Fatal("expected valid readings error, got nil")
	}
	if !strings.Contains(err.Error(), "not enough valid temperature readings") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPublishMQTTState(t *testing.T) {
	client := &fakeMQTTClient{publishToken: &fakeToken{waitResult: true}}

	err := publishMQTTState(client, "zigbee2mqtt/R/set", "ON")
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

	err := publishMQTTState(client, "zigbee2mqtt/R/set", "OFF")
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to publish") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPublishMQTTState_TokenError(t *testing.T) {
	client := &fakeMQTTClient{publishToken: &fakeToken{waitResult: true, err: errors.New("broker failed")}}

	err := publishMQTTState(client, "zigbee2mqtt/R/set", "OFF")
	if err == nil {
		t.Fatal("expected token error, got nil")
	}
	if !strings.Contains(err.Error(), "broker failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

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
	if client.subscribedTo != "zigbee2mqtt/R" {
		t.Fatalf("expected subscription to zigbee2mqtt/R, got %s", client.subscribedTo)
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
	if client.publishedTopic != "zigbee2mqtt/R/set" {
		t.Fatalf("expected publish topic zigbee2mqtt/R/set, got %s", client.publishedTopic)
	}
	if payload, _ := client.payload.(string); payload != `{"state": "ON"}` {
		t.Fatalf("unexpected published payload: %v", client.payload)
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

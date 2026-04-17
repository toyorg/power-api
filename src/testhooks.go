package powerapi

import (
	"context"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/ssh"
)

const (
	DefaultThresholdTemp = defaultThresholdTemp
	PollInterval         = pollInterval
)

type ShutdownDeps struct {
	IsPrinterFinished      func(baseURL string) (bool, error)
	GetCurrentExtruderTemp func(baseURL string) (int, error)
	SendSSHCommand         func(host, user, pass, command string) error
	IsHostReachable        func(host string) bool
	PublishMQTTState       func(topic, state string) error
	Sleep                  func(time.Duration)
	PollInterval           time.Duration
}

func LoadConfig() (*Config, error) {
	return loadConfig()
}

func GetenvString(key, defaultVal string) string {
	return getenvString(key, defaultVal)
}

func GetenvInt(key string, defaultVal int) int {
	return getenvInt(key, defaultVal)
}

func IsPrinterFinished(baseURL string) (bool, error) {
	return isPrinterFinished(baseURL)
}

func GetCurrentExtruderTemperature(baseURL string) (int, error) {
	return getCurrentExtruderTemperature(baseURL)
}

func PublishMQTTState(client mqtt.Client, topic, state string) error {
	return publishMQTTState(client, topic, state)
}

func NewMQTTClient(host, user, pass string) (mqtt.Client, error) {
	return newMQTTClient(host, user, pass)
}

func NewMQTTClientWithFactory(host, user, pass string, newClientFn func(*mqtt.ClientOptions) mqtt.Client) (mqtt.Client, error) {
	return newMQTTClientWithFactory(host, user, pass, newClientFn)
}

func GetMQTTState(ctx context.Context, client mqtt.Client, topic string) (string, error) {
	return getMQTTState(ctx, client, topic)
}

func OnMQTTConnectionLost(client mqtt.Client, err error) {
	onMQTTConnectionLost(client, err)
}

func OnMQTTConnect(client mqtt.Client) {
	onMQTTConnect(client)
}

func OnMQTTReconnecting(client mqtt.Client, opts *mqtt.ClientOptions) {
	onMQTTReconnecting(client, opts)
}

func NewGetPrinterStateHandler(client mqtt.Client, cfg *Config) gin.HandlerFunc {
	return handleGetPrinterState(client, cfg)
}

func NewPostPrinterControlHandler(client mqtt.Client, cfg *Config) gin.HandlerFunc {
	return handlePostPrinterControl(client, cfg)
}

func NewPostPrinterControlHandlerWithShutdown(client mqtt.Client, cfg *Config, shutdownFn func(*Config, ShutdownDeps) error) gin.HandlerFunc {
	if shutdownFn == nil {
		return handlePostPrinterControl(client, cfg)
	}

	return handlePostPrinterControlWithShutdown(client, cfg, func(c *Config, deps shutdownDeps) error {
		return shutdownFn(c, ShutdownDeps{
			IsPrinterFinished:      deps.isPrinterFinished,
			GetCurrentExtruderTemp: deps.getCurrentExtruderTemp,
			SendSSHCommand:         deps.sendSSHCommand,
			IsHostReachable:        deps.isHostReachable,
			PublishMQTTState:       deps.publishMQTTState,
			Sleep:                  deps.sleep,
			PollInterval:           deps.pollInterval,
		})
	})
}

func DefaultShutdownDeps(client mqtt.Client) ShutdownDeps {
	deps := defaultShutdownDeps(client)
	return ShutdownDeps{
		IsPrinterFinished:      deps.isPrinterFinished,
		GetCurrentExtruderTemp: deps.getCurrentExtruderTemp,
		SendSSHCommand:         deps.sendSSHCommand,
		IsHostReachable:        deps.isHostReachable,
		PublishMQTTState:       deps.publishMQTTState,
		Sleep:                  deps.sleep,
		PollInterval:           deps.pollInterval,
	}
}

func SendSSHCommand(host, user, pass, command string) error {
	return sendSSHCommand(host, user, pass, command)
}

func SendSSHCommandWithDial(host, user, pass, command string, dialFn func(network, addr string, config *ssh.ClientConfig) (SSHClient, error)) error {
	return sendSSHCommandWithDial(host, user, pass, command, dialFn)
}

func IsHostReachable(host string) bool {
	return isHostReachable(host)
}

func RunWithDeps(
	loadConfigFn func() (*Config, error),
	newMQTTClientFn func(host, user, pass string) (mqtt.Client, error),
	runServerFn func(*gin.Engine) error,
) error {
	return runWithDeps(loadConfigFn, newMQTTClientFn, runServerFn)
}

func SetRunDepsForTest(
	loadConfigFn func() (*Config, error),
	newMQTTClientFn func(host, user, pass string) (mqtt.Client, error),
	runServerFn func(*gin.Engine) error,
) func() {
	prevLoad := runLoadConfig
	prevMQTT := runNewMQTT
	prevServer := runHTTPServer

	if loadConfigFn != nil {
		runLoadConfig = loadConfigFn
	}
	if newMQTTClientFn != nil {
		runNewMQTT = newMQTTClientFn
	}
	if runServerFn != nil {
		runHTTPServer = runServerFn
	}

	return func() {
		runLoadConfig = prevLoad
		runNewMQTT = prevMQTT
		runHTTPServer = prevServer
	}
}

func ShutdownPrinter(cfg *Config, deps ShutdownDeps) error {
	internalDeps := shutdownDeps{
		isPrinterFinished:      deps.IsPrinterFinished,
		getCurrentExtruderTemp: deps.GetCurrentExtruderTemp,
		sendSSHCommand:         deps.SendSSHCommand,
		isHostReachable:        deps.IsHostReachable,
		publishMQTTState:       deps.PublishMQTTState,
		sleep:                  deps.Sleep,
		pollInterval:           deps.PollInterval,
	}

	return shutdownPrinter(cfg, internalDeps)
}

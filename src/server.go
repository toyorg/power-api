package powerapi

import (
	"fmt"
	"log"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/gin-gonic/gin"
)

var (
	runLoadConfig = loadConfig
	runNewMQTT    = newMQTTClient
	runHTTPServer = func(router *gin.Engine) error {
		return router.Run(serverPort)
	}
)

// Run loads configuration, initializes dependencies, and starts the HTTP server.
func Run() error {
	return runWithDeps(runLoadConfig, runNewMQTT, runHTTPServer)
}

func runWithDeps(
	loadConfigFn func() (*Config, error),
	newMQTTClientFn func(host, user, pass string) (mqtt.Client, error),
	runServerFn func(*gin.Engine) error,
) error {
	config, err := loadConfigFn()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	mqttClient, err := newMQTTClientFn(config.MQTTHost, config.MQTTUser, config.MQTTPass)
	if err != nil {
		return fmt.Errorf("failed to connect to MQTT: %w", err)
	}
	defer mqttClient.Disconnect(250)

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()

	router.GET("/api/3d-printer", handleGetPrinterState(mqttClient, config))
	router.POST("/api/3d-printer", handlePostPrinterControl(mqttClient, config))

	log.Printf("Starting server on %s", serverPort)
	if err := runServerFn(router); err != nil {
		return fmt.Errorf("server error: %w", err)
	}

	return nil
}

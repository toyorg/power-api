package powerapi

import (
	"context"
	"log"
	"net/http"
	"sync/atomic"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/gin-gonic/gin"
)

func handleGetPrinterState(client mqtt.Client, _ *Config) gin.HandlerFunc {
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
	return handlePostPrinterControlWithShutdown(client, cfg, shutdownPrinter)
}

func handlePostPrinterControlWithShutdown(client mqtt.Client, cfg *Config, shutdownFn func(*Config, shutdownDeps) error) gin.HandlerFunc {
	var shutdownInProgress atomic.Bool

	return func(c *gin.Context) {
		var req StateRequest
		if err := c.BindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		switch req.State {
		case "ON":
			if err := publishMQTTState(client, "zigbee2mqtt/R/set", "ON"); err != nil {
				log.Printf("failed to publish ON state: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to turn on printer"})
				return
			}
			c.JSON(http.StatusOK, StateResponse{State: "ON"})

		case "OFF":
			if !shutdownInProgress.CompareAndSwap(false, true) {
				c.JSON(http.StatusOK, StateResponse{State: "ON"})
				return
			}

			if err := shutdownFn(cfg, defaultShutdownDeps(client)); err != nil {
				log.Printf("shutdown error: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}

			defer shutdownInProgress.Store(false)

			c.JSON(http.StatusOK, StateResponse{State: "OFF"})

		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid state"})
		}
	}
}

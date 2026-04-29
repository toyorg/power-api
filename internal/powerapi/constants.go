package powerapi

import "time"

const (
	defaultThresholdTemp  = 49
	mqttConnectTimeout    = 10 * time.Second
	mqttConnectMaxRetries = 3
	mqttCommandTimeout    = 5 * time.Second
	pollInterval          = 5 * time.Second
	serverPort            = ":8000"

	topicPrinterState = "zigbee2mqtt/R"
	topicPrinterSet   = "zigbee2mqtt/R/set"

	stateON  = "ON"
	stateOFF = "OFF"
)

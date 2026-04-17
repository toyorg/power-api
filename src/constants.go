package powerapi

import "time"

const (
	// Safe temperature threshold for extruder to consider printer as finished. This is a safety measure to prevent shutting down the printer while it's still hot.
	defaultThresholdTemp = 49
	mqttConnectTimeout   = 10 * time.Second
	mqttCommandTimeout   = 5 * time.Second
	pollInterval         = 5 * time.Second
	serverPort           = ":8000"
)

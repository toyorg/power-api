package powerapi

// StateRequest represents a request to change printer state.
type StateRequest struct {
	State string `json:"state" binding:"required,oneof=ON OFF"`
}

// StateResponse represents the response with printer state.
type StateResponse struct {
	State string `json:"state"`
}

// Config holds application configuration.
type Config struct {
	MQTTHost      string
	MQTTUser      string
	MQTTPass      string
	SSHHost       string
	SSHUser       string
	SSHPass       string
	SSHHostPubKey string
	MoonrakerURL  string
	ThresholdTemp int
}

// printerStatusResponse represents the printer status from Moonraker API.
type printerStatusResponse struct {
	Result struct {
		Status struct {
			PrintStats struct {
				State string `json:"state"`
			} `json:"print_stats"`
		} `json:"status"`
	} `json:"result"`
}

// temperatureResponse represents the temperature data from Moonraker API.
type temperatureResponse struct {
	Result struct {
		Extruder struct {
			Temperatures []float64 `json:"temperatures"`
		} `json:"extruder"`
	} `json:"result"`
}

// mqttStateMessage represents the state message structure from MQTT.
type mqttStateMessage struct {
	State string `json:"state"`
}

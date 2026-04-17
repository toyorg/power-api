package powerapi

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// loadConfig loads the application configuration from environment variables.
func loadConfig() (*Config, error) {
	_ = godotenv.Load(".env")
	_ = godotenv.Load("/root/power-api/.env")

	return &Config{
		MQTTHost:      getenvString("mqtt_host", ""),
		MQTTUser:      getenvString("mqtt_user", ""),
		MQTTPass:      getenvString("mqtt_pass", ""),
		SSHHost:       getenvString("ssh_host", ""),
		SSHUser:       getenvString("ssh_user", ""),
		SSHPass:       getenvString("ssh_pass", ""),
		MoonrakerURL:  getenvString("moonraker_url", ""),
		ThresholdTemp: getenvInt("threshold_temp", defaultThresholdTemp),
	}, nil
}

// getenvString retrieves a string environment variable with a default value.
func getenvString(key, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return defaultVal
}

// getenvInt retrieves an integer environment variable with a default value.
func getenvInt(key string, defaultVal int) int {
	val, ok := os.LookupEnv(key)
	if !ok {
		return defaultVal
	}
	parsed, err := strconv.Atoi(val)
	if err != nil {
		return defaultVal
	}
	return parsed
}

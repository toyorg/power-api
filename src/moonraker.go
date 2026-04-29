package powerapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

var moonrakerHTTPClient = &http.Client{Timeout: 10 * time.Second}

// isPrinterFinished checks if the printer has finished printing.
func isPrinterFinished(baseURL string) (bool, error) {
	resp, err := moonrakerHTTPClient.Get(baseURL + "/printer/objects/query?print_stats")
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("unexpected status code %d from printer status", resp.StatusCode)
	}

	var result printerStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, fmt.Errorf("failed to decode printer status: %w", err)
	}

	state := result.Result.Status.PrintStats.State
	return state == "standby" || state == "complete", nil
}

// getCurrentExtruderTemperature fetches and calculates the average extruder temperature.
func getCurrentExtruderTemperature(baseURL string) (int, error) {
	resp, err := moonrakerHTTPClient.Get(baseURL + "/server/temperature_store")
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected status code %d from temperature store", resp.StatusCode)
	}

	var result temperatureResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("failed to decode temperature data: %w", err)
	}

	temps := result.Result.Extruder.Temperatures
	if len(temps) <= 10 {
		return 0, fmt.Errorf("insufficient temperature data: got %d samples", len(temps))
	}

	const maxSamples = 10
	if len(temps) > maxSamples {
		temps = temps[len(temps)-maxSamples:]
	}

	const minTemp = 1
	const maxTemp = 400
	var filtered []float64
	for _, v := range temps {
		if v >= minTemp && v <= maxTemp {
			filtered = append(filtered, v)
		}
	}

	const minValidReadings = 5
	if len(filtered) <= minValidReadings {
		return 0, fmt.Errorf("not enough valid temperature readings after filtering: %d", len(filtered))
	}

	sum := 0.0
	for _, v := range filtered {
		sum += v
	}
	avg := sum / float64(len(filtered))
	return int(avg), nil
}

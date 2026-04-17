package tests

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	powerapi "power-api/src"
)

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

			finished, err := powerapi.IsPrinterFinished(server.URL)
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

	_, err := powerapi.IsPrinterFinished(server.URL)
	if err == nil {
		t.Fatal("expected decode error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to decode printer status") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestIsPrinterFinished_HTTPError(t *testing.T) {
	_, err := powerapi.IsPrinterFinished("http://127.0.0.1:0")
	if err == nil {
		t.Fatal("expected HTTP error, got nil")
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

	temp, err := powerapi.GetCurrentExtruderTemperature(server.URL)
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

	_, err := powerapi.GetCurrentExtruderTemperature(server.URL)
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

	_, err := powerapi.GetCurrentExtruderTemperature(server.URL)
	if err == nil {
		t.Fatal("expected valid readings error, got nil")
	}
	if !strings.Contains(err.Error(), "not enough valid temperature readings") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetCurrentExtruderTemperature_DecodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("{"))
	}))
	defer server.Close()

	_, err := powerapi.GetCurrentExtruderTemperature(server.URL)
	if err == nil {
		t.Fatal("expected decode error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to decode temperature data") {
		t.Fatalf("unexpected error: %v", err)
	}
}

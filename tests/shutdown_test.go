package tests

import (
	"errors"
	"strings"
	"testing"
	"time"

	powerapi "power-api/src"
)

func TestShutdownPrinter_ResponsesReturnedNow(t *testing.T) {
	cfg := &powerapi.Config{
		MoonrakerURL:  "http://moonraker.local",
		SSHHost:       "printer-host",
		SSHUser:       "root",
		SSHPass:       "secret",
		ThresholdTemp: powerapi.DefaultThresholdTemp,
	}

	var (
		finishedCalls int
		tempCalls     int
		sshCalls      int
		publishCalls  int
		sleepCalls    int
	)

	deps := powerapi.ShutdownDeps{
		IsPrinterFinished: func(baseURL string) (bool, error) {
			finishedCalls++
			if baseURL != cfg.MoonrakerURL {
				t.Fatalf("unexpected moonraker URL: %s", baseURL)
			}
			return true, nil
		},
		GetCurrentExtruderTemp: func(baseURL string) (int, error) {
			tempCalls++
			if baseURL != cfg.MoonrakerURL {
				t.Fatalf("unexpected moonraker URL: %s", baseURL)
			}
			return cfg.ThresholdTemp - 1, nil
		},
		SendSSHCommand: func(host, user, pass, command string) error {
			sshCalls++
			if host != cfg.SSHHost || user != cfg.SSHUser || pass != cfg.SSHPass {
				t.Fatalf("unexpected ssh args: host=%s user=%s pass=%s", host, user, pass)
			}
			if command != "/sbin/shutdown 0" {
				t.Fatalf("unexpected ssh command: %s", command)
			}
			return nil
		},
		IsHostReachable: func(host string) bool {
			if host != cfg.SSHHost {
				t.Fatalf("unexpected host in reachability check: %s", host)
			}
			return false
		},
		PublishMQTTState: func(topic, state string) error {
			publishCalls++
			if topic != "zigbee2mqtt/R/set" || state != "OFF" {
				t.Fatalf("unexpected mqtt publish args: topic=%s state=%s", topic, state)
			}
			return nil
		},
		Sleep: func(time.Duration) {
			sleepCalls++
		},
		PollInterval: powerapi.PollInterval,
	}

	if err := powerapi.ShutdownPrinter(cfg, deps); err != nil {
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
	cfg := &powerapi.Config{
		MoonrakerURL:  "http://moonraker.local",
		SSHHost:       "printer-host",
		SSHUser:       "root",
		SSHPass:       "secret",
		ThresholdTemp: powerapi.DefaultThresholdTemp,
	}

	var (
		statusCalls  int
		tempCalls    int
		sshCalls     int
		publishCalls int
		sleepCalls   int
		elapsed      time.Duration
	)

	deps := powerapi.ShutdownDeps{
		IsPrinterFinished: func(string) (bool, error) {
			statusCalls++
			if elapsed < 30*time.Second {
				return false, errors.New("moonraker status not available yet")
			}
			return true, nil
		},
		GetCurrentExtruderTemp: func(string) (int, error) {
			tempCalls++
			if elapsed < 30*time.Second {
				return 0, errors.New("temperature endpoint not available yet")
			}
			return cfg.ThresholdTemp - 1, nil
		},
		SendSSHCommand: func(host, user, pass, command string) error {
			sshCalls++
			return nil
		},
		IsHostReachable: func(string) bool {
			return false
		},
		PublishMQTTState: func(topic, state string) error {
			publishCalls++
			return nil
		},
		Sleep: func(d time.Duration) {
			sleepCalls++
			elapsed += d
		},
		PollInterval: powerapi.PollInterval,
	}

	if err := powerapi.ShutdownPrinter(cfg, deps); err != nil {
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

func TestShutdownPrinter_ReturnsErrorWhenPublishFails(t *testing.T) {
	cfg := &powerapi.Config{
		MoonrakerURL:  "http://moonraker.local",
		SSHHost:       "printer-host",
		SSHUser:       "root",
		SSHPass:       "secret",
		ThresholdTemp: powerapi.DefaultThresholdTemp,
	}

	deps := powerapi.ShutdownDeps{
		IsPrinterFinished: func(string) (bool, error) { return true, nil },
		GetCurrentExtruderTemp: func(string) (int, error) {
			return cfg.ThresholdTemp - 1, nil
		},
		SendSSHCommand:   func(string, string, string, string) error { return nil },
		IsHostReachable:  func(string) bool { return false },
		PublishMQTTState: func(string, string) error { return errors.New("mqtt down") },
		Sleep:            func(time.Duration) {},
		PollInterval:     powerapi.PollInterval,
	}

	err := powerapi.ShutdownPrinter(cfg, deps)
	if err == nil {
		t.Fatal("expected shutdown error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to publish OFF state") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestShutdownPrinter_DefaultsSleepAndPollInterval(t *testing.T) {
	cfg := &powerapi.Config{
		MoonrakerURL:  "http://moonraker.local",
		SSHHost:       "printer-host",
		SSHUser:       "root",
		SSHPass:       "secret",
		ThresholdTemp: powerapi.DefaultThresholdTemp,
	}

	deps := powerapi.ShutdownDeps{
		IsPrinterFinished: func(string) (bool, error) { return true, nil },
		GetCurrentExtruderTemp: func(string) (int, error) {
			return cfg.ThresholdTemp - 1, nil
		},
		SendSSHCommand: func(string, string, string, string) error { return nil },
		IsHostReachable: func(string) bool {
			return false
		},
		PublishMQTTState: func(string, string) error {
			return nil
		},
		Sleep:        nil,
		PollInterval: 0,
	}

	err := powerapi.ShutdownPrinter(cfg, deps)
	if err != nil {
		t.Fatalf("shutdownPrinter returned error: %v", err)
	}
}

func TestShutdownPrinter_ContinuesWhenSSHCommandFails(t *testing.T) {
	cfg := &powerapi.Config{
		MoonrakerURL:  "http://moonraker.local",
		SSHHost:       "printer-host",
		SSHUser:       "root",
		SSHPass:       "secret",
		ThresholdTemp: powerapi.DefaultThresholdTemp,
	}

	var publishCalls int

	deps := powerapi.ShutdownDeps{
		IsPrinterFinished:      func(string) (bool, error) { return true, nil },
		GetCurrentExtruderTemp: func(string) (int, error) { return cfg.ThresholdTemp - 1, nil },
		SendSSHCommand:         func(string, string, string, string) error { return errors.New("ssh failure") },
		IsHostReachable:        func(string) bool { return false },
		PublishMQTTState: func(string, string) error {
			publishCalls++
			return nil
		},
		Sleep:        func(time.Duration) {},
		PollInterval: powerapi.PollInterval,
	}

	err := powerapi.ShutdownPrinter(cfg, deps)
	if err != nil {
		t.Fatalf("shutdownPrinter returned error: %v", err)
	}
	if publishCalls != 1 {
		t.Fatalf("expected relay publish despite ssh failure, got %d", publishCalls)
	}
}

func TestShutdownPrinter_RetriesWhenTempReadFails(t *testing.T) {
	cfg := &powerapi.Config{
		MoonrakerURL:  "http://moonraker.local",
		SSHHost:       "printer-host",
		SSHUser:       "root",
		SSHPass:       "secret",
		ThresholdTemp: powerapi.DefaultThresholdTemp,
	}

	var (
		tempCalls  int
		sleepCalls int
	)

	deps := powerapi.ShutdownDeps{
		IsPrinterFinished: func(string) (bool, error) { return true, nil },
		GetCurrentExtruderTemp: func(string) (int, error) {
			tempCalls++
			if tempCalls == 1 {
				return 0, errors.New("temp endpoint flaky")
			}
			return cfg.ThresholdTemp - 1, nil
		},
		SendSSHCommand:   func(string, string, string, string) error { return nil },
		IsHostReachable:  func(string) bool { return false },
		PublishMQTTState: func(string, string) error { return nil },
		Sleep: func(time.Duration) {
			sleepCalls++
		},
		PollInterval: powerapi.PollInterval,
	}

	err := powerapi.ShutdownPrinter(cfg, deps)
	if err != nil {
		t.Fatalf("shutdownPrinter returned error: %v", err)
	}
	if tempCalls != 2 {
		t.Fatalf("expected 2 temp reads, got %d", tempCalls)
	}
	if sleepCalls != 1 {
		t.Fatalf("expected 1 retry sleep, got %d", sleepCalls)
	}
}

func TestShutdownPrinter_WaitsUntilHostBecomesUnreachable(t *testing.T) {
	cfg := &powerapi.Config{
		MoonrakerURL:  "http://moonraker.local",
		SSHHost:       "printer-host",
		SSHUser:       "root",
		SSHPass:       "secret",
		ThresholdTemp: powerapi.DefaultThresholdTemp,
	}

	var (
		reachableChecks int
		sleepCalls      int
	)

	deps := powerapi.ShutdownDeps{
		IsPrinterFinished:      func(string) (bool, error) { return true, nil },
		GetCurrentExtruderTemp: func(string) (int, error) { return cfg.ThresholdTemp - 1, nil },
		SendSSHCommand:         func(string, string, string, string) error { return nil },
		IsHostReachable: func(string) bool {
			reachableChecks++
			return reachableChecks < 3
		},
		PublishMQTTState: func(string, string) error { return nil },
		Sleep: func(time.Duration) {
			sleepCalls++
		},
		PollInterval: powerapi.PollInterval,
	}

	err := powerapi.ShutdownPrinter(cfg, deps)
	if err != nil {
		t.Fatalf("shutdownPrinter returned error: %v", err)
	}
	if reachableChecks != 3 {
		t.Fatalf("expected 3 host checks, got %d", reachableChecks)
	}
	if sleepCalls != 2 {
		t.Fatalf("expected 2 sleeps while waiting for host down, got %d", sleepCalls)
	}
}

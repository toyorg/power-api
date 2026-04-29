package powerapi

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestShutdownPrinter_ResponsesReturnedNow(t *testing.T) {
	cfg := &Config{
		MoonrakerURL:  "http://moonraker.local",
		SSHHost:       "printer-host",
		SSHUser:       "root",
		SSHPass:       "secret",
		SSHHostPubKey: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGfakefakefakefakefakefakefakefake test@local",
		ThresholdTemp: defaultThresholdTemp,
	}

	var (
		finishedCalls int
		tempCalls     int
		sshCalls      int
		publishCalls  int
		sleepCalls    int
	)

	deps := shutdownDeps{
		isPrinterFinished: func(baseURL string) (bool, error) {
			finishedCalls++
			if baseURL != cfg.MoonrakerURL {
				t.Fatalf("unexpected moonraker URL: %s", baseURL)
			}
			return true, nil
		},
		getCurrentExtruderTemp: func(baseURL string) (int, error) {
			tempCalls++
			if baseURL != cfg.MoonrakerURL {
				t.Fatalf("unexpected moonraker URL: %s", baseURL)
			}
			return cfg.ThresholdTemp - 1, nil
		},
		sendSSHCommand: func(host, user, pass, hostPublicKey, command string) error {
			sshCalls++
			if host != cfg.SSHHost || user != cfg.SSHUser || pass != cfg.SSHPass {
				t.Fatalf("unexpected ssh args: host=%s user=%s pass=%s", host, user, pass)
			}
			if hostPublicKey != cfg.SSHHostPubKey {
				t.Fatalf("unexpected ssh host key: %s", hostPublicKey)
			}
			if command != "/sbin/shutdown 0" {
				t.Fatalf("unexpected ssh command: %s", command)
			}
			return nil
		},
		isHostReachable: func(host string) bool {
			if host != cfg.SSHHost {
				t.Fatalf("unexpected host in reachability check: %s", host)
			}
			return false
		},
		publishMQTTState: func(topic, state string) error {
			publishCalls++
			if topic != topicPrinterSet || state != stateOFF {
				t.Fatalf("unexpected mqtt publish args: topic=%s state=%s", topic, state)
			}
			return nil
		},
		sleep: func(time.Duration) {
			sleepCalls++
		},
		pollInterval: pollInterval,
	}

	if err := shutdownPrinter(cfg, deps); err != nil {
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
	cfg := &Config{
		MoonrakerURL:  "http://moonraker.local",
		SSHHost:       "printer-host",
		SSHUser:       "root",
		SSHPass:       "secret",
		SSHHostPubKey: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGfakefakefakefakefakefakefakefake test@local",
		ThresholdTemp: defaultThresholdTemp,
	}

	var (
		statusCalls  int
		tempCalls    int
		sshCalls     int
		publishCalls int
		sleepCalls   int
		elapsed      time.Duration
	)

	deps := shutdownDeps{
		isPrinterFinished: func(string) (bool, error) {
			statusCalls++
			if elapsed < 30*time.Second {
				return false, errors.New("moonraker status not available yet")
			}
			return true, nil
		},
		getCurrentExtruderTemp: func(string) (int, error) {
			tempCalls++
			if elapsed < 30*time.Second {
				return 0, errors.New("temperature endpoint not available yet")
			}
			return cfg.ThresholdTemp - 1, nil
		},
		sendSSHCommand: func(host, user, pass, hostPublicKey, command string) error {
			sshCalls++
			return nil
		},
		isHostReachable: func(string) bool {
			return false
		},
		publishMQTTState: func(topic, state string) error {
			publishCalls++
			return nil
		},
		sleep: func(d time.Duration) {
			sleepCalls++
			elapsed += d
		},
		pollInterval: pollInterval,
	}

	if err := shutdownPrinter(cfg, deps); err != nil {
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
	cfg := &Config{
		MoonrakerURL:  "http://moonraker.local",
		SSHHost:       "printer-host",
		SSHUser:       "root",
		SSHPass:       "secret",
		SSHHostPubKey: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGfakefakefakefakefakefakefakefake test@local",
		ThresholdTemp: defaultThresholdTemp,
	}

	deps := shutdownDeps{
		isPrinterFinished:      func(string) (bool, error) { return true, nil },
		getCurrentExtruderTemp: func(string) (int, error) { return cfg.ThresholdTemp - 1, nil },
		sendSSHCommand:         func(string, string, string, string, string) error { return nil },
		isHostReachable:        func(string) bool { return false },
		publishMQTTState:       func(string, string) error { return errors.New("mqtt down") },
		sleep:                  func(time.Duration) {},
		pollInterval:           pollInterval,
	}

	err := shutdownPrinter(cfg, deps)
	if err == nil {
		t.Fatal("expected shutdown error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to publish OFF state") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestShutdownPrinter_DefaultsSleepAndPollInterval(t *testing.T) {
	cfg := &Config{
		MoonrakerURL:  "http://moonraker.local",
		SSHHost:       "printer-host",
		SSHUser:       "root",
		SSHPass:       "secret",
		SSHHostPubKey: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGfakefakefakefakefakefakefakefake test@local",
		ThresholdTemp: defaultThresholdTemp,
	}

	deps := shutdownDeps{
		isPrinterFinished:      func(string) (bool, error) { return true, nil },
		getCurrentExtruderTemp: func(string) (int, error) { return cfg.ThresholdTemp - 1, nil },
		sendSSHCommand:         func(string, string, string, string, string) error { return nil },
		isHostReachable:        func(string) bool { return false },
		publishMQTTState:       func(string, string) error { return nil },
		sleep:                  nil,
		pollInterval:           0,
	}

	err := shutdownPrinter(cfg, deps)
	if err != nil {
		t.Fatalf("shutdownPrinter returned error: %v", err)
	}
}

func TestShutdownPrinter_ContinuesWhenSSHCommandFails(t *testing.T) {
	cfg := &Config{
		MoonrakerURL:  "http://moonraker.local",
		SSHHost:       "printer-host",
		SSHUser:       "root",
		SSHPass:       "secret",
		SSHHostPubKey: "ecdsa-sha2-nistp256 AAAAC3NzaC1lZDI1NTE5AAAAIGfakefakefakefakefakefakefakefake",
		ThresholdTemp: defaultThresholdTemp,
	}

	var publishCalls int

	deps := shutdownDeps{
		isPrinterFinished:      func(string) (bool, error) { return true, nil },
		getCurrentExtruderTemp: func(string) (int, error) { return cfg.ThresholdTemp - 1, nil },
		sendSSHCommand:         func(string, string, string, string, string) error { return errors.New("ssh failure") },
		isHostReachable:        func(string) bool { return false },
		publishMQTTState: func(string, string) error {
			publishCalls++
			return nil
		},
		sleep:        func(time.Duration) {},
		pollInterval: pollInterval,
	}

	err := shutdownPrinter(cfg, deps)
	if err != nil {
		t.Fatalf("shutdownPrinter returned error: %v", err)
	}
	if publishCalls != 1 {
		t.Fatalf("expected relay publish despite ssh failure, got %d", publishCalls)
	}
}

func TestShutdownPrinter_RetriesWhenTempReadFails(t *testing.T) {
	cfg := &Config{
		MoonrakerURL:  "http://moonraker.local",
		SSHHost:       "printer-host",
		SSHUser:       "root",
		SSHPass:       "secret",
		SSHHostPubKey: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGfakefakefakefakefakefakefakefake test@local",
		ThresholdTemp: defaultThresholdTemp,
	}

	var (
		tempCalls  int
		sleepCalls int
	)

	deps := shutdownDeps{
		isPrinterFinished: func(string) (bool, error) { return true, nil },
		getCurrentExtruderTemp: func(string) (int, error) {
			tempCalls++
			if tempCalls == 1 {
				return 0, errors.New("temp endpoint flaky")
			}
			return cfg.ThresholdTemp - 1, nil
		},
		sendSSHCommand:   func(string, string, string, string, string) error { return nil },
		isHostReachable:  func(string) bool { return false },
		publishMQTTState: func(string, string) error { return nil },
		sleep: func(time.Duration) {
			sleepCalls++
		},
		pollInterval: pollInterval,
	}

	err := shutdownPrinter(cfg, deps)
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
	cfg := &Config{
		MoonrakerURL:  "http://moonraker.local",
		SSHHost:       "printer-host",
		SSHUser:       "root",
		SSHPass:       "secret",
		SSHHostPubKey: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGfakefakefakefakefakefakefakefake test@local",
		ThresholdTemp: defaultThresholdTemp,
	}

	var (
		reachableChecks int
		sleepCalls      int
	)

	deps := shutdownDeps{
		isPrinterFinished:      func(string) (bool, error) { return true, nil },
		getCurrentExtruderTemp: func(string) (int, error) { return cfg.ThresholdTemp - 1, nil },
		sendSSHCommand:         func(string, string, string, string, string) error { return nil },
		isHostReachable: func(string) bool {
			reachableChecks++
			return reachableChecks < 3
		},
		publishMQTTState: func(string, string) error { return nil },
		sleep: func(time.Duration) {
			sleepCalls++
		},
		pollInterval: pollInterval,
	}

	err := shutdownPrinter(cfg, deps)
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

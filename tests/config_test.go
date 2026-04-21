package tests

import (
	"testing"

	powerapi "power-api/src"
)

func TestGetenvStringAndGetenvInt(t *testing.T) {
	t.Setenv("power_api_test_str", "abc")
	t.Setenv("power_api_test_int", "123")
	t.Setenv("power_api_test_bad_int", "bad")

	if got := powerapi.GetenvString("power_api_test_str", "fallback"); got != "abc" {
		t.Fatalf("expected getenvString to return env value, got %q", got)
	}
	if got := powerapi.GetenvString("power_api_test_missing_str", "fallback"); got != "fallback" {
		t.Fatalf("expected getenvString fallback, got %q", got)
	}

	if got := powerapi.GetenvInt("power_api_test_int", 7); got != 123 {
		t.Fatalf("expected getenvInt parsed value 123, got %d", got)
	}
	if got := powerapi.GetenvInt("power_api_test_bad_int", 7); got != 7 {
		t.Fatalf("expected getenvInt fallback for bad value, got %d", got)
	}
	if got := powerapi.GetenvInt("power_api_test_missing_int", 7); got != 7 {
		t.Fatalf("expected getenvInt fallback for missing value, got %d", got)
	}
}

func TestLoadConfig(t *testing.T) {
	t.Setenv("mqtt_host", "broker.local")
	t.Setenv("mqtt_user", "user")
	t.Setenv("mqtt_pass", "pass")
	t.Setenv("ssh_host", "ssh.local")
	t.Setenv("ssh_user", "root")
	t.Setenv("ssh_pass", "secret")
	t.Setenv("ssh_host_public_key", "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGfakefakefakefakefakefakefakefake test@local")
	t.Setenv("moonraker_url", "http://moonraker.local")
	t.Setenv("threshold_temp", "55")

	cfg, err := powerapi.LoadConfig()
	if err != nil {
		t.Fatalf("loadConfig returned error: %v", err)
	}

	if cfg.MQTTHost != "broker.local" || cfg.MQTTUser != "user" || cfg.MQTTPass != "pass" {
		t.Fatalf("unexpected MQTT config: %+v", cfg)
	}
	if cfg.SSHHost != "ssh.local" || cfg.SSHUser != "root" || cfg.SSHPass != "secret" || cfg.SSHHostPubKey == "" {
		t.Fatalf("unexpected SSH config: %+v", cfg)
	}
	if cfg.MoonrakerURL != "http://moonraker.local" {
		t.Fatalf("unexpected moonraker URL: %s", cfg.MoonrakerURL)
	}
	if cfg.ThresholdTemp != 55 {
		t.Fatalf("unexpected threshold temperature: %d", cfg.ThresholdTemp)
	}
}

package tests

import (
	"errors"
	"strings"
	"testing"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/ssh"
	powerapi "power-api/src"
)

type fakeSSHSession struct {
	runErr error
	runs   int
}

func (s *fakeSSHSession) Run(string) error {
	s.runs++
	return s.runErr
}

func (s *fakeSSHSession) Close() error { return nil }

type fakeSSHClient struct {
	session      powerapi.SSHSession
	newSessionErr error
	newCalls     int
}

func (c *fakeSSHClient) NewSession() (powerapi.SSHSession, error) {
	c.newCalls++
	if c.newSessionErr != nil {
		return nil, c.newSessionErr
	}
	return c.session, nil
}

func (c *fakeSSHClient) Close() error { return nil }

func TestNewMQTTClientWithFactory_Success(t *testing.T) {
	fakeClient := &fakeMQTTClient{connectToken: &fakeToken{waitResult: true}}

	client, err := powerapi.NewMQTTClientWithFactory("broker.local", "user", "pass", func(_ *mqtt.ClientOptions) mqtt.Client {
		return fakeClient
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if client == nil {
		t.Fatal("expected mqtt client, got nil")
	}
}

func TestNewMQTTClientWithFactory_Timeout(t *testing.T) {
	fakeClient := &fakeMQTTClient{connectToken: &fakeToken{waitResult: false}}

	_, err := powerapi.NewMQTTClientWithFactory("broker.local", "user", "pass", func(_ *mqtt.ClientOptions) mqtt.Client {
		return fakeClient
	})
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "MQTT connection timeout") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewMQTTClientWithFactory_ConnectError(t *testing.T) {
	fakeClient := &fakeMQTTClient{connectToken: &fakeToken{waitResult: true, err: errors.New("auth failed")}}

	_, err := powerapi.NewMQTTClientWithFactory("broker.local", "user", "pass", func(_ *mqtt.ClientOptions) mqtt.Client {
		return fakeClient
	})
	if err == nil {
		t.Fatal("expected connect error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to connect to MQTT broker") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewMQTTClient_WrapperError(t *testing.T) {
	_, err := powerapi.NewMQTTClient("127.0.0.1", "user", "pass")
	if err == nil {
		t.Fatal("expected mqtt wrapper error, got nil")
	}
}

func TestRun_WrapperSuccessViaInjectedDeps(t *testing.T) {
	fakeClient := &fakeMQTTClient{}
	restore := powerapi.SetRunDepsForTest(
		func() (*powerapi.Config, error) {
			return &powerapi.Config{MQTTHost: "broker.local", MQTTUser: "u", MQTTPass: "p"}, nil
		},
		func(string, string, string) (mqtt.Client, error) {
			return fakeClient, nil
		},
		func(*gin.Engine) error { return nil },
	)
	defer restore()

	err := powerapi.Run()
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if fakeClient.disconnects != 1 {
		t.Fatalf("expected disconnect to be called once, got %d", fakeClient.disconnects)
	}
}

func TestRun_WrapperServerErrorViaInjectedDeps(t *testing.T) {
	restore := powerapi.SetRunDepsForTest(
		func() (*powerapi.Config, error) {
			return &powerapi.Config{MQTTHost: "broker.local", MQTTUser: "u", MQTTPass: "p"}, nil
		},
		func(string, string, string) (mqtt.Client, error) {
			return &fakeMQTTClient{}, nil
		},
		func(*gin.Engine) error { return errors.New("listen failed") },
	)
	defer restore()

	err := powerapi.Run()
	if err == nil {
		t.Fatal("expected server error, got nil")
	}
	if !strings.Contains(err.Error(), "server error") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunWithDeps_LoadConfigError(t *testing.T) {
	err := powerapi.RunWithDeps(
		func() (*powerapi.Config, error) { return nil, errors.New("env invalid") },
		func(string, string, string) (mqtt.Client, error) { return &fakeMQTTClient{}, nil },
		func(*gin.Engine) error { return nil },
	)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to load configuration") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRun_WrapperMQTTError(t *testing.T) {
	t.Setenv("mqtt_host", "127.0.0.1")
	t.Setenv("mqtt_user", "user")
	t.Setenv("mqtt_pass", "pass")

	err := powerapi.Run()
	if err == nil {
		t.Fatal("expected run error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to connect to MQTT") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunWithDeps_MQTTError(t *testing.T) {
	err := powerapi.RunWithDeps(
		func() (*powerapi.Config, error) {
			return &powerapi.Config{MQTTHost: "broker.local", MQTTUser: "u", MQTTPass: "p"}, nil
		},
		func(string, string, string) (mqtt.Client, error) { return nil, errors.New("mqtt down") },
		func(*gin.Engine) error { return nil },
	)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to connect to MQTT") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunWithDeps_ServerErrorAndDisconnect(t *testing.T) {
	fakeClient := &fakeMQTTClient{}
	err := powerapi.RunWithDeps(
		func() (*powerapi.Config, error) {
			return &powerapi.Config{MQTTHost: "broker.local", MQTTUser: "u", MQTTPass: "p"}, nil
		},
		func(string, string, string) (mqtt.Client, error) { return fakeClient, nil },
		func(*gin.Engine) error { return errors.New("bind failed") },
	)
	if err == nil {
		t.Fatal("expected server error, got nil")
	}
	if !strings.Contains(err.Error(), "server error") {
		t.Fatalf("unexpected error: %v", err)
	}
	if fakeClient.disconnects != 1 {
		t.Fatalf("expected disconnect to be called once, got %d", fakeClient.disconnects)
	}
}

func TestRunWithDeps_SuccessAndDisconnect(t *testing.T) {
	fakeClient := &fakeMQTTClient{}
	err := powerapi.RunWithDeps(
		func() (*powerapi.Config, error) {
			return &powerapi.Config{MQTTHost: "broker.local", MQTTUser: "u", MQTTPass: "p"}, nil
		},
		func(string, string, string) (mqtt.Client, error) { return fakeClient, nil },
		func(*gin.Engine) error { return nil },
	)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if fakeClient.disconnects != 1 {
		t.Fatalf("expected disconnect to be called once, got %d", fakeClient.disconnects)
	}
}

func TestSendSSHCommand_InvalidHostReturnsError(t *testing.T) {
	err := powerapi.SendSSHCommand("127.0.0.1", "root", "secret", "echo ok")
	if err == nil {
		t.Fatal("expected ssh error on localhost:22, got nil")
	}
}

func TestSendSSHCommandWithDial_DialError(t *testing.T) {
	err := powerapi.SendSSHCommandWithDial("printer-host", "root", "secret", "echo ok", func(string, string, *ssh.ClientConfig) (powerapi.SSHClient, error) {
		return nil, errors.New("dial failed")
	})
	if err == nil {
		t.Fatal("expected dial error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to connect via SSH") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSendSSHCommandWithDial_NewSessionError(t *testing.T) {
	err := powerapi.SendSSHCommandWithDial("printer-host", "root", "secret", "echo ok", func(string, string, *ssh.ClientConfig) (powerapi.SSHClient, error) {
		return &fakeSSHClient{newSessionErr: errors.New("session failed")}, nil
	})
	if err == nil {
		t.Fatal("expected session error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to create SSH session") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSendSSHCommandWithDial_SuccessEvenWhenRunErrors(t *testing.T) {
	s := &fakeSSHSession{runErr: errors.New("remote failed")}
	c := &fakeSSHClient{session: s}

	err := powerapi.SendSSHCommandWithDial("printer-host", "root", "secret", "echo ok", func(string, string, *ssh.ClientConfig) (powerapi.SSHClient, error) {
		return c, nil
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if c.newCalls != 1 {
		t.Fatalf("expected NewSession to be called once, got %d", c.newCalls)
	}
	if s.runs != 1 {
		t.Fatalf("expected Run to be called once, got %d", s.runs)
	}
}

func TestIsHostReachable_InvalidHost(t *testing.T) {
	if got := powerapi.IsHostReachable("definitely-not-a-real-host.invalid"); got {
		t.Fatal("expected unreachable host to return false")
	}
}

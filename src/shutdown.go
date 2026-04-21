package powerapi

import (
	"fmt"
	"log"
	"os/exec"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"golang.org/x/crypto/ssh"
)

type SSHSession interface {
	Run(cmd string) error
	Close() error
}

type SSHClient interface {
	NewSession() (SSHSession, error)
	Close() error
}

type sshClientAdapter struct {
	client *ssh.Client
}

func (a *sshClientAdapter) NewSession() (SSHSession, error) {
	return a.client.NewSession()
}

func (a *sshClientAdapter) Close() error {
	return a.client.Close()
}

type sshDialFunc func(network, addr string, config *ssh.ClientConfig) (SSHClient, error)

var dialSSH sshDialFunc = func(network, addr string, config *ssh.ClientConfig) (SSHClient, error) {
	client, err := ssh.Dial(network, addr, config)
	if err != nil {
		return nil, err
	}
	return &sshClientAdapter{client: client}, nil
}

type shutdownDeps struct {
	isPrinterFinished      func(baseURL string) (bool, error)
	getCurrentExtruderTemp func(baseURL string) (int, error)
	sendSSHCommand         func(host, user, pass, hostPublicKey, command string) error
	isHostReachable        func(host string) bool
	publishMQTTState       func(topic, state string) error
	sleep                  func(time.Duration)
	pollInterval           time.Duration
}

func defaultShutdownDeps(client mqtt.Client) shutdownDeps {
	return shutdownDeps{
		isPrinterFinished:      isPrinterFinished,
		getCurrentExtruderTemp: getCurrentExtruderTemperature,
		sendSSHCommand:         sendSSHCommand,
		isHostReachable:        isHostReachable,
		publishMQTTState: func(topic, state string) error {
			return publishMQTTState(client, topic, state)
		},
		sleep:        time.Sleep,
		pollInterval: pollInterval,
	}
}

func shutdownPrinter(cfg *Config, deps shutdownDeps) error {
	if deps.pollInterval <= 0 {
		deps.pollInterval = pollInterval
	}
	if deps.sleep == nil {
		deps.sleep = time.Sleep
	}

	for {
		finished, err := deps.isPrinterFinished(cfg.MoonrakerURL)
		if err != nil {
			log.Printf("error checking printer status: %v", err)
			deps.sleep(deps.pollInterval)
			continue
		}

		temp, err := deps.getCurrentExtruderTemp(cfg.MoonrakerURL)
		if err != nil {
			log.Printf("error getting temperature: %v", err)
			deps.sleep(deps.pollInterval)
			continue
		}

		log.Printf("Printer finished: %v, Current temp: %d°C, threshold: %d°C", finished, temp, cfg.ThresholdTemp)

		if finished && temp < cfg.ThresholdTemp {
			break
		}
		deps.sleep(deps.pollInterval)
	}

	if err := deps.sendSSHCommand(cfg.SSHHost, cfg.SSHUser, cfg.SSHPass, cfg.SSHHostPubKey, "/sbin/shutdown 0"); err != nil {
		log.Printf("failed to send shutdown command: %v", err)
	}

	for deps.isHostReachable(cfg.SSHHost) {
		deps.sleep(deps.pollInterval)
	}

	if err := deps.publishMQTTState("zigbee2mqtt/R/set", "OFF"); err != nil {
		return fmt.Errorf("failed to publish OFF state: %w", err)
	}

	return nil
}

// sendSSHCommand executes a command on a remote SSH host.
func sendSSHCommand(host, user, pass, hostPublicKey, command string) error {
	return sendSSHCommandWithDial(host, user, pass, hostPublicKey, command, dialSSH)
}

func sendSSHCommandWithDial(host, user, pass, hostPublicKey, command string, dialFn sshDialFunc) error {
	pubKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(hostPublicKey))

	if err != nil {
		return fmt.Errorf("failed to parse SSH host public key: %w", err)
	}

	config := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.Password(pass)},
		HostKeyCallback: ssh.FixedHostKey(pubKey),
	}

	client, err := dialFn("tcp", fmt.Sprintf("%s:22", host), config)
	if err != nil {
		return fmt.Errorf("failed to connect via SSH: %w", err)
	}
	defer func() { _ = client.Close() }()

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create SSH session: %w", err)
	}
	defer func() { _ = session.Close() }()

	// Intentionally ignore the error from session.Run because the SSH session may be terminated
	// before the command completes due to immediate shutdown of the remote host.
	_ = session.Run(command)

	return nil
}

// isHostReachable checks if a host is reachable via ping.
func isHostReachable(host string) bool {
	cmd := exec.Command("ping", "-c", "1", host)
	return cmd.Run() == nil
}

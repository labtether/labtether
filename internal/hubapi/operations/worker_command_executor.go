package operations

import (
	"context"
	"encoding/base64"
	"fmt"
	"net"
	neturl "net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/securityruntime"
	"github.com/labtether/labtether/internal/terminal"
)

const (
	ExecutorModeSimulated = "simulated"
	ExecutorModeSSH       = "ssh"
	ExecutorModeLocal     = "local"
)

// CommandExecutorConfig holds configuration for executing terminal commands.
type CommandExecutorConfig struct {
	Mode           string
	Timeout        time.Duration
	MaxOutputBytes int
}

// SSHTarget holds parsed SSH target connection info.
type SSHTarget struct {
	User string
	Host string
	Port int
}

// ExecuteCommand runs a terminal command using the configured executor mode.
func ExecuteCommand(job terminal.CommandJob) terminal.CommandResult {
	cfg := LoadCommandExecutorConfig()
	status, output := ExecuteConfiguredCommand(job, cfg)

	return terminal.CommandResult{
		JobID:       job.JobID,
		SessionID:   job.SessionID,
		CommandID:   job.CommandID,
		Status:      status,
		Output:      output,
		CompletedAt: time.Now().UTC(),
	}
}

// LoadCommandExecutorConfig reads the command executor configuration from environment.
func LoadCommandExecutorConfig() CommandExecutorConfig {
	mode := strings.ToLower(strings.TrimSpace(shared.EnvOrDefault("TERMINAL_EXECUTOR_MODE", ExecutorModeSimulated)))
	switch mode {
	case ExecutorModeSSH, ExecutorModeLocal:
	default:
		mode = ExecutorModeSimulated
	}

	return CommandExecutorConfig{
		Mode:           mode,
		Timeout:        shared.EnvOrDefaultDuration("TERMINAL_COMMAND_TIMEOUT", 30*time.Second),
		MaxOutputBytes: shared.EnvOrDefaultInt("TERMINAL_MAX_OUTPUT_BYTES", 64*1024),
	}
}

// ExecuteConfiguredCommand runs a terminal command using the given config.
func ExecuteConfiguredCommand(job terminal.CommandJob, cfg CommandExecutorConfig) (string, string) {
	switch cfg.Mode {
	case ExecutorModeSSH:
		output, err := ExecuteSSHCommand(job, cfg)
		if err != nil {
			return "failed", OutputForError(output, err)
		}
		return "succeeded", output
	case ExecutorModeLocal:
		output, err := ExecuteLocalCommand(job, cfg)
		if err != nil {
			return "failed", OutputForError(output, err)
		}
		return "succeeded", output
	default:
		return ExecuteSimulatedCommand(job)
	}
}

// ExecuteSimulatedCommand simulates command execution for testing.
func ExecuteSimulatedCommand(job terminal.CommandJob) (string, string) {
	status := "succeeded"
	output := fmt.Sprintf("simulated execution on %s: %s", job.Target, job.Command)
	if strings.Contains(strings.ToLower(job.Command), "fail") {
		status = "failed"
		output = "simulated command failure"
	}
	return status, output
}

// ExecuteLocalCommand runs a command locally via shell.
func ExecuteLocalCommand(job terminal.CommandJob, cfg CommandExecutorConfig) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	if err := securityruntime.ValidateShellCommand(job.Command); err != nil {
		return "", err
	}
	cmd, err := securityruntime.NewCommandContext(ctx, "sh", "-lc", job.Command)
	if err != nil {
		return "", err
	}
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return TruncateOutput(output, cfg.MaxOutputBytes), fmt.Errorf("command timed out after %s", cfg.Timeout)
	}
	return TruncateOutput(output, cfg.MaxOutputBytes), err
}

// ExecuteSSHCommand runs a command on a remote host via SSH.
func ExecuteSSHCommand(job terminal.CommandJob, cfg CommandExecutorConfig) (string, error) {
	sshConfig, err := ResolveJobSSHConfig(job)
	if err != nil {
		return "", err
	}

	authMethods, err := ResolveSSHAuthMethods(sshConfig)
	if err != nil {
		return "", err
	}

	hostKeyCallback, err := BuildSSHHostKeyCallback(sshConfig)
	if err != nil {
		return "", err
	}

	clientConfig := &ssh.ClientConfig{
		User:            sshConfig.User,
		Auth:            authMethods,
		HostKeyCallback: hostKeyCallback,
		Timeout:         cfg.Timeout,
	}

	if err := securityruntime.ValidateOutboundDialTarget(sshConfig.Host, sshConfig.Port); err != nil {
		return "", err
	}
	addr := net.JoinHostPort(sshConfig.Host, strconv.Itoa(sshConfig.Port))
	client, err := ssh.Dial("tcp", addr, clientConfig)
	if err != nil {
		return "", fmt.Errorf("ssh dial failed: %w", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("ssh session failed: %w", err)
	}
	defer session.Close()

	type runResult struct {
		out []byte
		err error
	}
	resultCh := make(chan runResult, 1)
	go func() {
		out, runErr := session.CombinedOutput(job.Command)
		resultCh <- runResult{out: out, err: runErr}
	}()

	timer := time.NewTimer(cfg.Timeout)
	defer timer.Stop()

	select {
	case result := <-resultCh:
		return TruncateOutput(result.out, cfg.MaxOutputBytes), result.err
	case <-timer.C:
		_ = session.Close()
		_ = client.Close()
		return "", fmt.Errorf("ssh command timed out after %s", cfg.Timeout)
	}
}

// ResolveJobSSHConfig resolves SSH configuration from a command job.
func ResolveJobSSHConfig(job terminal.CommandJob) (*terminal.SSHConfig, error) {
	defaultUser := strings.TrimSpace(shared.EnvOrDefault("SSH_USERNAME", ""))
	defaultPort := shared.EnvOrDefaultInt("SSH_PORT", 22)
	if defaultPort <= 0 {
		defaultPort = 22
	}

	if job.SSHConfig != nil && strings.TrimSpace(job.SSHConfig.Host) != "" {
		resolved := &terminal.SSHConfig{
			Host:                 strings.TrimSpace(job.SSHConfig.Host),
			Port:                 job.SSHConfig.Port,
			User:                 strings.TrimSpace(job.SSHConfig.User),
			Password:             strings.TrimSpace(job.SSHConfig.Password),
			PrivateKey:           strings.TrimSpace(job.SSHConfig.PrivateKey),
			PrivateKeyPassphrase: strings.TrimSpace(job.SSHConfig.PrivateKeyPassphrase),
			StrictHostKey:        job.SSHConfig.StrictHostKey,
			HostKey:              strings.TrimSpace(job.SSHConfig.HostKey),
		}
		if resolved.Port <= 0 {
			resolved.Port = defaultPort
		}
		if resolved.User == "" {
			resolved.User = defaultUser
		}
		if resolved.User == "" {
			return nil, fmt.Errorf("ssh user is required")
		}
		return resolved, nil
	}

	target, err := ParseSSHTarget(job.Target, defaultUser, defaultPort)
	if err != nil {
		return nil, err
	}
	return &terminal.SSHConfig{
		Host: target.Host,
		Port: target.Port,
		User: target.User,
	}, nil
}

// BuildSSHHostKeyCallback constructs an SSH host key callback based on config.
func BuildSSHHostKeyCallback(cfg *terminal.SSHConfig) (ssh.HostKeyCallback, error) {
	strict := shared.EnvOrDefaultBool("SSH_STRICT_HOST_KEY", true)
	expected := strings.TrimSpace(shared.EnvOrDefault("SSH_HOST_KEY", ""))

	if cfg != nil {
		if cfg.StrictHostKey {
			strict = true
			expected = strings.TrimSpace(cfg.HostKey)
		} else if strings.TrimSpace(cfg.HostKey) != "" {
			expected = strings.TrimSpace(cfg.HostKey)
		}
	}

	if !strict {
		// #nosec G106 -- explicit non-strict host-key mode for local/dev operator flows.
		return ssh.InsecureIgnoreHostKey(), nil
	}
	if expected == "" {
		knownHostsCallback, err := shared.BuildKnownHostsHostKeyCallback()
		if err != nil {
			return nil, fmt.Errorf("strict host key enabled but no SSH_HOST_KEY provided and no known_hosts file is available")
		}
		return knownHostsCallback, nil
	}

	return func(_ string, _ net.Addr, key ssh.PublicKey) error {
		fingerprint := strings.TrimSpace(ssh.FingerprintSHA256(key))
		if strings.EqualFold(fingerprint, expected) {
			return nil
		}

		encoded := base64.StdEncoding.EncodeToString(key.Marshal())
		if strings.EqualFold(encoded, expected) {
			return nil
		}

		return fmt.Errorf("host key mismatch")
	}, nil
}

// ResolveSSHAuthMethods resolves SSH authentication methods from config and environment.
func ResolveSSHAuthMethods(cfg *terminal.SSHConfig) ([]ssh.AuthMethod, error) {
	auth := make([]ssh.AuthMethod, 0, 3)
	if cfg != nil {
		if password := strings.TrimSpace(cfg.Password); password != "" {
			auth = append(auth, ssh.Password(password))
		}
	}

	parseSigner := func(keyRaw, passphrase string) error {
		keyRaw = shared.NormalizePrivateKey(keyRaw)
		if strings.TrimSpace(keyRaw) == "" {
			return nil
		}

		var (
			signer ssh.Signer
			err    error
		)
		if strings.TrimSpace(passphrase) != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase([]byte(keyRaw), []byte(passphrase))
		} else {
			signer, err = ssh.ParsePrivateKey([]byte(keyRaw))
		}
		if err != nil {
			return fmt.Errorf("invalid ssh private key: %w", err)
		}
		auth = append(auth, ssh.PublicKeys(signer))
		return nil
	}

	if cfg != nil && strings.TrimSpace(cfg.PrivateKey) != "" {
		if err := parseSigner(cfg.PrivateKey, cfg.PrivateKeyPassphrase); err != nil {
			return nil, err
		}
	}

	if password := strings.TrimSpace(shared.EnvOrDefault("SSH_PASSWORD", "")); password != "" {
		auth = append(auth, ssh.Password(password))
	}

	privateKeyPassphrase := strings.TrimSpace(shared.EnvOrDefault("SSH_PRIVATE_KEY_PASSPHRASE", ""))
	if keyB64 := strings.TrimSpace(shared.EnvOrDefault("SSH_PRIVATE_KEY_B64", "")); keyB64 != "" {
		decoded, err := base64.StdEncoding.DecodeString(keyB64)
		if err != nil {
			return nil, fmt.Errorf("invalid SSH_PRIVATE_KEY_B64: %w", err)
		}
		if err := parseSigner(string(decoded), privateKeyPassphrase); err != nil {
			return nil, err
		}
	}

	if keyRaw := strings.TrimSpace(shared.EnvOrDefault("SSH_PRIVATE_KEY", "")); keyRaw != "" {
		if err := parseSigner(keyRaw, privateKeyPassphrase); err != nil {
			return nil, err
		}
	}

	if keyPath := strings.TrimSpace(shared.EnvOrDefault("SSH_PRIVATE_KEY_PATH", "")); keyPath != "" {
		payload, err := os.ReadFile(keyPath) // #nosec G304 -- Path comes from trusted operator runtime env for worker SSH auth.
		if err != nil {
			return nil, fmt.Errorf("unable to read SSH_PRIVATE_KEY_PATH: %w", err)
		}
		if err := parseSigner(string(payload), privateKeyPassphrase); err != nil {
			return nil, err
		}
	}

	if len(auth) == 0 {
		return nil, fmt.Errorf("no ssh auth method configured")
	}

	return auth, nil
}

// ParseSSHTarget parses an SSH target string into its components.
func ParseSSHTarget(raw, defaultUser string, defaultPort int) (SSHTarget, error) {
	target := strings.TrimSpace(raw)
	if target == "" {
		return SSHTarget{}, fmt.Errorf("target is required")
	}

	if defaultPort <= 0 {
		defaultPort = 22
	}

	out := SSHTarget{
		User: strings.TrimSpace(defaultUser),
		Port: defaultPort,
	}

	if strings.HasPrefix(strings.ToLower(target), "ssh://") {
		parsed, err := neturl.Parse(target)
		if err != nil {
			return SSHTarget{}, fmt.Errorf("invalid ssh target: %w", err)
		}
		out.Host = strings.TrimSpace(parsed.Hostname())
		if parsed.Port() != "" {
			port, err := strconv.Atoi(parsed.Port())
			if err != nil || port <= 0 {
				return SSHTarget{}, fmt.Errorf("invalid ssh target port")
			}
			out.Port = port
		}
		if parsed.User != nil {
			out.User = strings.TrimSpace(parsed.User.Username())
		}
	} else {
		hostPart := target
		if at := strings.LastIndex(target, "@"); at > 0 {
			out.User = strings.TrimSpace(target[:at])
			hostPart = target[at+1:]
		}
		if strings.TrimSpace(hostPart) == "" {
			return SSHTarget{}, fmt.Errorf("invalid ssh target host")
		}
		if host, port, err := net.SplitHostPort(hostPart); err == nil {
			out.Host = strings.TrimSpace(host)
			portInt, convErr := strconv.Atoi(strings.TrimSpace(port))
			if convErr != nil || portInt <= 0 {
				return SSHTarget{}, fmt.Errorf("invalid ssh target port")
			}
			out.Port = portInt
		} else {
			out.Host = strings.TrimSpace(hostPart)
		}
	}

	if out.Host == "" {
		return SSHTarget{}, fmt.Errorf("ssh host is required")
	}
	if out.User == "" {
		return SSHTarget{}, fmt.Errorf("ssh user is required")
	}
	if out.Port <= 0 {
		out.Port = 22
	}
	return out, nil
}

// TruncateOutput truncates output to the given max bytes.
func TruncateOutput(payload []byte, maxBytes int) string {
	if maxBytes <= 0 {
		maxBytes = 8 * 1024
	}
	if len(payload) <= maxBytes {
		return strings.TrimSpace(string(payload))
	}

	prefix := strings.TrimSpace(string(payload[:maxBytes]))
	return fmt.Sprintf("%s\n...output truncated (%d bytes total)", prefix, len(payload))
}

// OutputForError combines output with an error message.
func OutputForError(output string, err error) string {
	trimmedOutput := strings.TrimSpace(output)
	if err == nil {
		return trimmedOutput
	}
	if trimmedOutput == "" {
		return err.Error()
	}
	return fmt.Sprintf("%s\nerror: %s", trimmedOutput, err.Error())
}

package protocols

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/labtether/labtether/internal/securityruntime"
)

const (
	dialTimeout      = 6 * time.Second
	handshakeTimeout = 8 * time.Second
)

// TestSSH attempts a full SSH dial, handshake, and authentication against the
// target host:port. At least one of password or privateKey must be non-empty.
func TestSSH(ctx context.Context, host string, port int, username, password, privateKey string, hostKeyCallback ssh.HostKeyCallback) *TestResult {
	start := time.Now()

	authMethods := make([]ssh.AuthMethod, 0, 2)
	if password := strings.TrimSpace(password); password != "" {
		authMethods = append(authMethods, ssh.Password(password))
	}
	if keyRaw := strings.TrimSpace(privateKey); keyRaw != "" {
		signer, err := ssh.ParsePrivateKey([]byte(keyRaw))
		if err != nil {
			return &TestResult{
				Success:   false,
				LatencyMs: time.Since(start).Milliseconds(),
				Error:     fmt.Sprintf("invalid private key: %v", err),
			}
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	}

	if len(authMethods) == 0 {
		return &TestResult{
			Success:   false,
			LatencyMs: time.Since(start).Milliseconds(),
			Error:     "no auth method configured: provide password or private key",
		}
	}

	if hostKeyCallback == nil {
		return &TestResult{
			Success:   false,
			LatencyMs: time.Since(start).Milliseconds(),
			Error:     "SSH host key callback is required",
		}
	}

	cfg := &ssh.ClientConfig{
		User:            strings.TrimSpace(username),
		Auth:            authMethods,
		HostKeyCallback: hostKeyCallback,
		Timeout:         dialTimeout,
	}

	dialCtx, cancel := context.WithTimeout(ctx, handshakeTimeout)
	defer cancel()

	client, err := securityruntime.DialOutboundSSHContext(dialCtx, host, port, cfg, dialTimeout)
	if err != nil {
		return &TestResult{
			Success:   false,
			LatencyMs: time.Since(start).Milliseconds(),
			Error:     "connection failed: " + err.Error(),
		}
	}
	defer client.Close()

	return &TestResult{
		Success:   true,
		LatencyMs: time.Since(start).Milliseconds(),
	}
}

// TestTelnet dials the host:port and waits for any banner data to confirm the
// service is responding.
func TestTelnet(ctx context.Context, host string, port int) *TestResult {
	start := time.Now()

	dialCtx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()

	conn, err := securityruntime.DialOutboundTCPContext(dialCtx, host, port, dialTimeout)
	if err != nil {
		return &TestResult{
			Success:   false,
			LatencyMs: time.Since(start).Milliseconds(),
			Error:     err.Error(),
		}
	}
	defer conn.Close()

	// Wait up to handshakeTimeout for the server to send any banner data.
	deadline := time.Now().Add(handshakeTimeout)
	if err := conn.SetReadDeadline(deadline); err != nil {
		return &TestResult{
			Success:   false,
			LatencyMs: time.Since(start).Milliseconds(),
			Error:     fmt.Sprintf("failed to set read deadline: %v", err),
		}
	}

	buf := make([]byte, 64)
	n, err := conn.Read(buf)
	if err != nil && n == 0 {
		return &TestResult{
			Success:   false,
			LatencyMs: time.Since(start).Milliseconds(),
			Error:     fmt.Sprintf("no banner received: %v", err),
		}
	}

	return &TestResult{
		Success:   true,
		LatencyMs: time.Since(start).Milliseconds(),
	}
}

// TestVNC dials host:port and reads the RFB version string to confirm a VNC
// server is present.
func TestVNC(ctx context.Context, host string, port int) *TestResult {
	start := time.Now()

	dialCtx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()

	conn, err := securityruntime.DialOutboundTCPContext(dialCtx, host, port, dialTimeout)
	if err != nil {
		return &TestResult{
			Success:   false,
			LatencyMs: time.Since(start).Milliseconds(),
			Error:     err.Error(),
		}
	}
	defer conn.Close()

	if err := conn.SetReadDeadline(time.Now().Add(handshakeTimeout)); err != nil {
		return &TestResult{
			Success:   false,
			LatencyMs: time.Since(start).Milliseconds(),
			Error:     fmt.Sprintf("failed to set read deadline: %v", err),
		}
	}

	// RFB protocol version string: "RFB XXX.YYY\n" (12 bytes).
	buf := make([]byte, 12)
	n, err := conn.Read(buf)
	if err != nil || n < 3 {
		errMsg := "no RFB version string received"
		if err != nil {
			errMsg = fmt.Sprintf("read error: %v", err)
		}
		return &TestResult{
			Success:   false,
			LatencyMs: time.Since(start).Milliseconds(),
			Error:     errMsg,
		}
	}

	if !strings.HasPrefix(string(buf[:n]), "RFB") {
		return &TestResult{
			Success:   false,
			LatencyMs: time.Since(start).Milliseconds(),
			Error:     fmt.Sprintf("unexpected server banner: %q", string(buf[:n])),
		}
	}

	return &TestResult{
		Success:   true,
		LatencyMs: time.Since(start).Milliseconds(),
	}
}

// TestRDP dials host:port to verify TCP reachability. If guacdAddr is
// non-empty, the result notes that guacd is available (but the connection is
// not proxied through guacd during the test). If guacdAddr is empty, the
// result succeeds but notes that credentials were not verified.
func TestRDP(ctx context.Context, host string, port int, guacdAddr string) *TestResult {
	start := time.Now()

	dialCtx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()

	conn, err := securityruntime.DialOutboundTCPContext(dialCtx, host, port, dialTimeout)
	if err != nil {
		return &TestResult{
			Success:   false,
			LatencyMs: time.Since(start).Milliseconds(),
			Error:     err.Error(),
		}
	}
	if err := conn.Close(); err != nil {
		return &TestResult{
			Success:   false,
			LatencyMs: time.Since(start).Milliseconds(),
			Error:     fmt.Sprintf("close test connection: %v", err),
		}
	}

	if guacdAddr != "" {
		guacdDialCtx, guacdCancel := context.WithTimeout(ctx, dialTimeout)
		defer guacdCancel()
		guacdHost, guacdPortRaw, splitErr := net.SplitHostPort(strings.TrimSpace(guacdAddr))
		guacdPort, portErr := strconv.Atoi(guacdPortRaw)
		if splitErr != nil || portErr != nil {
			return &TestResult{
				Success:   false,
				LatencyMs: time.Since(start).Milliseconds(),
				Error:     "invalid guacd address",
			}
		}
		guacdConn, guacdErr := securityruntime.DialOutboundTCPContext(guacdDialCtx, guacdHost, guacdPort, dialTimeout)
		if guacdErr != nil {
			return &TestResult{
				Success:   false,
				LatencyMs: time.Since(start).Milliseconds(),
				Error:     "guacd unavailable: " + guacdErr.Error(),
			}
		}
		if closeErr := guacdConn.Close(); closeErr != nil {
			return &TestResult{
				Success:   false,
				LatencyMs: time.Since(start).Milliseconds(),
				Error:     fmt.Sprintf("close guacd test connection: %v", closeErr),
			}
		}
		return &TestResult{
			Success:   true,
			LatencyMs: time.Since(start).Milliseconds(),
			Message:   "reachable (guacd reachable)",
		}
	}

	return &TestResult{
		Success:   true,
		LatencyMs: time.Since(start).Milliseconds(),
		Message:   "reachable (credentials not verified — guacd not available)",
	}
}

// TestARD delegates to TestVNC because Apple Remote Desktop uses the RFB
// protocol on the same port.
func TestARD(ctx context.Context, host string, port int) *TestResult {
	return TestVNC(ctx, host, port)
}

package protocols

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

const (
	dialTimeout      = 6 * time.Second
	handshakeTimeout = 8 * time.Second
)

// TestSSH attempts a full SSH dial, handshake, and authentication against the
// target host:port. At least one of password or privateKey must be non-empty.
func TestSSH(ctx context.Context, host string, port int, username, password, privateKey string, hostKeyCallback ssh.HostKeyCallback) *TestResult {
	start := time.Now()

	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))

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
		// #nosec G106 -- test-only connectivity probe; operator is aware that
		// host key validation is skipped during reachability tests.
		hostKeyCallback = ssh.InsecureIgnoreHostKey()
	}

	cfg := &ssh.ClientConfig{
		User:            strings.TrimSpace(username),
		Auth:            authMethods,
		HostKeyCallback: hostKeyCallback,
		Timeout:         dialTimeout,
	}

	dialCtx, cancel := context.WithTimeout(ctx, handshakeTimeout)
	defer cancel()

	conn, err := (&net.Dialer{}).DialContext(dialCtx, "tcp", addr)
	if err != nil {
		return &TestResult{
			Success:   false,
			LatencyMs: time.Since(start).Milliseconds(),
			Error:     "connection failed: " + err.Error(),
		}
	}
	defer conn.Close()

	sshConn, chans, reqs, err := ssh.NewClientConn(conn, addr, cfg)
	if err != nil {
		return &TestResult{
			Success:   false,
			LatencyMs: time.Since(start).Milliseconds(),
			Error:     "SSH handshake failed: " + err.Error(),
		}
	}
	client := ssh.NewClient(sshConn, chans, reqs)
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

	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))

	dialCtx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()

	var d net.Dialer
	conn, err := d.DialContext(dialCtx, "tcp", addr)
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

	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))

	dialCtx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()

	var d net.Dialer
	conn, err := d.DialContext(dialCtx, "tcp", addr)
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

	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))

	dialCtx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()

	var d net.Dialer
	conn, err := d.DialContext(dialCtx, "tcp", addr)
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

	latency := time.Since(start).Milliseconds()

	if guacdAddr != "" {
		return &TestResult{
			Success:   true,
			LatencyMs: latency,
			Message:   "reachable (guacd available)",
		}
	}

	return &TestResult{
		Success:   true,
		LatencyMs: latency,
		Message:   "reachable (credentials not verified — guacd not available)",
	}
}

// TestARD delegates to TestVNC because Apple Remote Desktop uses the RFB
// protocol on the same port.
func TestARD(ctx context.Context, host string, port int) *TestResult {
	return TestVNC(ctx, host, port)
}

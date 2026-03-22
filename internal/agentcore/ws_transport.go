package agentcore

import (
	"context"
	cryptorand "crypto/rand"
	"crypto/tls"
	"fmt"
	"log"
	"math"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/agentmgr"
)

// Error classification for WebSocket connect failures.
type connectErrorKind int

const (
	errKindNone      connectErrorKind = iota
	errKindAuth                       // 401/403 — credentials rejected
	errKindTransient                  // network, DNS, server error, etc.
)

func (k connectErrorKind) String() string {
	switch k {
	case errKindNone:
		return "none"
	case errKindAuth:
		return "auth_failed"
	case errKindTransient:
		return "transient"
	default:
		return "unknown"
	}
}

// classifyConnectError inspects the error and HTTP response from a WebSocket
// dial attempt and returns the error kind.
func classifyConnectError(err error, resp *http.Response) connectErrorKind {
	if err == nil {
		return errKindNone
	}
	if resp != nil {
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return errKindAuth
		}
	}
	return errKindTransient
}

func jitterDuration(max time.Duration) time.Duration {
	if max <= 0 {
		return 0
	}
	n, err := cryptorand.Int(cryptorand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return 0
	}
	return time.Duration(n.Int64())
}

// Reconnect backoff constants.
const (
	maxBackoff           = 60 * time.Second
	authBackoff          = 5 * time.Minute
	authFailureThreshold = 3
)

// Client-side keepalive constants. The agent client pings every 25s
// (staggered from the hub's 30s interval). If no pong (or any other
// frame) arrives within 60s the read deadline fires and the connection
// is torn down.
//
// Named "client" to avoid collision with the hub-side clientPingInterval
// (30s) in cmd/labtether/agent_ws_handler.go.
const (
	clientPingInterval = 25 * time.Second
	clientReadDeadline = 60 * time.Second
)

type wsTransport struct {
	url            string
	token          string
	assetID        string
	platform       string
	agentVersion   string
	tlsConfig      *tls.Config
	tokenFilePath  string
	deviceIdentity *deviceIdentity

	// Diagnostic counters — accessed with sync/atomic.
	messagesSent     int64
	messagesReceived int64
	reconnectCount   int64

	startedAt time.Time

	mu                      sync.Mutex
	conn                    *websocket.Conn
	connected               bool
	pingDone                chan struct{} // closed to stop ping goroutine
	consecutiveAuthFailures int
	lastError               string
	lastErrorAt             time.Time
	disconnectedAt          time.Time

	networkChanged <-chan struct{} // signaled when local IPs change

	reEnrollFn     func() (string, error) // returns new token or error
	lastReEnrollAt time.Time

	connectWithResponseFn func(context.Context) (*http.Response, error)
	timeAfter             func(time.Duration) <-chan time.Time
	now                   func() time.Time
	jitter                func(time.Duration) time.Duration
}

// updateToken updates the bearer token and resets auth failure state.
func (t *wsTransport) updateToken(token string) {
	t.mu.Lock()
	t.token = token
	t.consecutiveAuthFailures = 0
	t.lastError = ""
	t.mu.Unlock()
}

// AssetID returns the asset ID associated with this transport.
func (t *wsTransport) AssetID() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.assetID
}

func newWSTransport(url, token, assetID, platform, agentVersion string, tlsConfig *tls.Config, tokenFilePath string, identity *deviceIdentity) *wsTransport {
	return &wsTransport{
		url:            normalizeWSBaseURL(url),
		token:          token,
		assetID:        assetID,
		platform:       platform,
		agentVersion:   strings.TrimSpace(agentVersion),
		tlsConfig:      tlsConfig,
		tokenFilePath:  tokenFilePath,
		deviceIdentity: identity,
		startedAt:      time.Now(),
		timeAfter:      time.After,
		now:            time.Now,
		jitter:         jitterDuration,
	}
}

func (t *wsTransport) connectAttempt(ctx context.Context) (*http.Response, error) {
	if t.connectWithResponseFn != nil {
		return t.connectWithResponseFn(ctx)
	}
	return t.connectWithResponse(ctx)
}

func (t *wsTransport) after(d time.Duration) <-chan time.Time {
	if t.timeAfter != nil {
		return t.timeAfter(d)
	}
	return time.After(d)
}

func (t *wsTransport) currentTime() time.Time {
	if t.now != nil {
		return t.now()
	}
	return time.Now()
}

func (t *wsTransport) jitterDuration(max time.Duration) time.Duration {
	if t.jitter != nil {
		return t.jitter(max)
	}
	return jitterDuration(max)
}

// connectWithResponse dials the hub and returns the HTTP response alongside the
// error so callers can inspect the status code for error classification.
func (t *wsTransport) connectWithResponse(ctx context.Context) (*http.Response, error) {
	if err := validateWebSocketTransportURL(t.url); err != nil {
		return nil, err
	}
	t.mu.Lock()
	token := t.token
	t.mu.Unlock()

	header := http.Header{}
	if token != "" {
		header.Set("Authorization", "Bearer "+token)
	} else {
		header.Set("X-Request-Enrollment", "true")
		hostname, _ := os.Hostname()
		header.Set("X-Hostname", hostname)
		if t.deviceIdentity != nil {
			header.Set("X-Device-Fingerprint", t.deviceIdentity.Fingerprint)
			header.Set("X-Device-Key-Alg", t.deviceIdentity.KeyAlgorithm)
			header.Set("X-Device-Public-Key", t.deviceIdentity.PublicKeyBase64)
		}
	}
	header.Set("X-Asset-ID", t.assetID)
	header.Set("X-Platform", t.platform)
	if t.agentVersion != "" {
		header.Set("X-Agent-Version", t.agentVersion)
	}

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		TLSClientConfig:  t.tlsConfig,
	}

	conn, resp, err := dialer.DialContext(ctx, t.url, header)
	if err != nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		return resp, err
	}

	// Set initial read deadline; the pong handler resets it on each pong.
	_ = conn.SetReadDeadline(time.Now().Add(clientReadDeadline))
	conn.SetPongHandler(func(_ string) error {
		return conn.SetReadDeadline(time.Now().Add(clientReadDeadline))
	})

	t.mu.Lock()
	if t.conn != nil {
		_ = t.conn.Close()
	}
	// Stop the previous ping goroutine if one is running.
	if t.pingDone != nil {
		close(t.pingDone)
	}
	pingDone := make(chan struct{})
	t.pingDone = pingDone
	t.conn = conn
	t.connected = true
	t.consecutiveAuthFailures = 0
	t.lastError = ""
	t.disconnectedAt = time.Time{}
	t.mu.Unlock()

	go t.pingLoop(conn, pingDone)

	log.Printf("agentws: connected to %s", t.url)
	return resp, nil
}

// Connect dials the hub WebSocket endpoint. Wraps connectWithResponse,
// discarding the HTTP response.
func (t *wsTransport) Connect(ctx context.Context) error {
	_, err := t.connectWithResponse(ctx)
	return err
}

// pingLoop sends periodic WebSocket pings to the hub. It exits when the done
// channel is closed or when the connection changes (reconnect replaces conn).
func (t *wsTransport) pingLoop(conn *websocket.Conn, done chan struct{}) {
	ticker := time.NewTicker(clientPingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			t.mu.Lock()
			if t.conn != conn {
				t.mu.Unlock()
				return
			}
			err := conn.WriteControl(
				websocket.PingMessage, nil,
				time.Now().Add(10*time.Second),
			)
			t.mu.Unlock()
			if err != nil {
				log.Printf("agentws: ping send failed: %v", err)
				t.markDisconnected()
				return
			}
		}
	}
}

func (t *wsTransport) Send(msg agentmgr.Message) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.conn == nil {
		return errNotConnected
	}
	// Apply a write deadline so that a slow or unresponsive hub cannot block
	// Send indefinitely while holding t.mu. Without this, heartbeat, telemetry,
	// VNC, terminal, and Docker-event goroutines would all serialize and stall
	// behind a single slow write.
	_ = t.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	err := t.conn.WriteJSON(msg)
	if err == nil {
		atomic.AddInt64(&t.messagesSent, 1)
	}
	return err
}

func (t *wsTransport) Receive() (agentmgr.Message, error) {
	t.mu.Lock()
	conn := t.conn
	t.mu.Unlock()
	if conn == nil {
		return agentmgr.Message{}, errNotConnected
	}

	var msg agentmgr.Message
	err := conn.ReadJSON(&msg)
	if err == nil {
		atomic.AddInt64(&t.messagesReceived, 1)
	}
	return msg, err
}

func (t *wsTransport) Close() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.pingDone != nil {
		close(t.pingDone)
		t.pingDone = nil
	}
	if t.conn != nil {
		_ = t.conn.Close()
		t.conn = nil
	}
	t.connected = false
}

func (t *wsTransport) Connected() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.connected
}

// ConnectionState returns a human-readable connection state plus the last error
// string and the time the transport was first disconnected.
func (t *wsTransport) ConnectionState() (state string, lastErr string, disconnectedAt time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.connected {
		return "connected", "", time.Time{}
	}
	if t.lastError == "auth_failed" {
		return "auth_failed", t.lastError, t.disconnectedAt
	}
	if t.lastError != "" {
		return "connecting", t.lastError, t.disconnectedAt
	}
	return "disconnected", "", t.disconnectedAt
}

func (t *wsTransport) markDisconnected() {
	t.mu.Lock()
	t.connected = false
	if t.pingDone != nil {
		close(t.pingDone)
		t.pingDone = nil
	}
	if t.disconnectedAt.IsZero() {
		t.disconnectedAt = time.Now().UTC()
	}
	if t.conn != nil {
		_ = t.conn.Close()
		t.conn = nil
	}
	t.mu.Unlock()
}

// reconnectLoop attempts to maintain a persistent WebSocket connection with
// exponential backoff (1s, 2s, 4s, 8s... cap 60s) and jitter. Auth failures
// (401/403) back off to 5-minute intervals after 3 consecutive failures.
func (t *wsTransport) reconnectLoop(ctx context.Context, onConnect func()) {
	backoff := time.Second

	defer func() {
		// Ensure state reflects "disconnected" (not stuck on "connecting")
		// when the reconnect loop exits.
		t.mu.Lock()
		t.lastError = ""
		t.mu.Unlock()
		t.markDisconnected()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if t.Connected() {
			// Wait a bit before checking again.
			select {
			case <-ctx.Done():
				return
			case <-t.after(time.Second):
				continue
			}
		}

		resp, err := t.connectAttempt(ctx)
		if err != nil {
			kind := classifyConnectError(err, resp)
			now := t.currentTime()

			t.mu.Lock()
			t.lastError = kind.String()
			t.lastErrorAt = now
			if t.disconnectedAt.IsZero() {
				t.disconnectedAt = now
			}

			var wait time.Duration
			if kind == errKindAuth {
				t.consecutiveAuthFailures++
				failures := t.consecutiveAuthFailures
				reEnroll := t.reEnrollFn
				lastReEnroll := t.lastReEnrollAt
				if failures >= authFailureThreshold {
					wait = authBackoff
					t.mu.Unlock()
					log.Printf("agentws: AUTH FAILURE (%d consecutive) — credentials rejected by hub, backing off to %s: %v",
						failures, wait, err)

					// Attempt re-enrollment if available and not too recent.
					if reEnroll != nil && t.currentTime().Sub(lastReEnroll) > 10*time.Minute {
						log.Printf("agentws: attempting re-enrollment after %d auth failures", failures)
						if newToken, reErr := reEnroll(); reErr == nil {
							t.updateToken(newToken)
							t.mu.Lock()
							t.lastReEnrollAt = t.currentTime().UTC()
							t.mu.Unlock()
							log.Printf("agentws: re-enrollment succeeded, retrying connection")
							backoff = time.Second
							continue
						} else {
							log.Printf("agentws: re-enrollment failed: %v", reErr)
							t.mu.Lock()
							t.lastReEnrollAt = t.currentTime().UTC()
							t.mu.Unlock()
						}
					}
				} else {
					t.mu.Unlock()
					jitter := t.jitterDuration(backoff / 4)
					wait = backoff + jitter
					log.Printf("agentws: auth failure (%d/%d), retrying in %s: %v",
						failures, authFailureThreshold, wait, err)
					backoff = time.Duration(math.Min(float64(backoff*2), float64(maxBackoff)))
				}
			} else {
				t.mu.Unlock()
				jitter := t.jitterDuration(backoff / 4)
				wait = backoff + jitter
				log.Printf("agentws: connect failed, retrying in %s: %v", wait, err)
				if backoff == time.Second && isTLSTrustError(err) {
					log.Printf("agentws: TLS certificate trust failed. Configure LABTETHER_TLS_CA_FILE with the hub CA, or temporarily set LABTETHER_TLS_SKIP_VERIFY=true for bootstrap only.")
				}
				backoff = time.Duration(math.Min(float64(backoff*2), float64(maxBackoff)))
			}

			select {
			case <-ctx.Done():
				return
			case <-t.after(wait):
			}
			continue
		}

		// Connected — reset backoff, record the reconnect, and notify.
		backoff = time.Second
		atomic.AddInt64(&t.reconnectCount, 1)
		if onConnect != nil {
			onConnect()
		}

		// Block on receive loop (handled elsewhere); just wait for disconnect.
		// The receive loop in the runtime will call markDisconnected on error.
		// Also listen for network changes to force an immediate reconnect.
		netCh := t.networkChanged
		if netCh == nil {
			netCh = make(chan struct{}) // never fires
		}
		for t.Connected() {
			select {
			case <-ctx.Done():
				return
			case <-netCh:
				log.Printf("agentws: network change — forcing reconnect")
				t.markDisconnected()
			case <-t.after(time.Second):
			}
		}
	}
}

// Stats returns a snapshot of the transport's diagnostic counters and uptime.
// All counter reads use atomic loads and are safe to call from any goroutine.
func (t *wsTransport) Stats() (sent, received, reconnects int64, uptime time.Duration) {
	return atomic.LoadInt64(&t.messagesSent),
		atomic.LoadInt64(&t.messagesReceived),
		atomic.LoadInt64(&t.reconnectCount),
		time.Since(t.startedAt)
}

type errNotConnectedType struct{}

func (errNotConnectedType) Error() string { return "websocket not connected" }

var errNotConnected error = errNotConnectedType{}

func validateWebSocketTransportURL(raw string) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return fmt.Errorf("websocket url is required")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || strings.TrimSpace(parsed.Host) == "" {
		return fmt.Errorf("invalid websocket url %q", raw)
	}
	switch strings.ToLower(strings.TrimSpace(parsed.Scheme)) {
	case "wss":
		return nil
	case "ws":
		if allowInsecureTransportOptIn() {
			return nil
		}
		return fmt.Errorf("insecure websocket scheme requires %s=true", envAllowInsecureTransport)
	default:
		return fmt.Errorf("unsupported websocket scheme %q", parsed.Scheme)
	}
}

func isTLSTrustError(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	if !strings.Contains(lower, "x509:") {
		return false
	}
	return strings.Contains(lower, "unknown authority") ||
		strings.Contains(lower, "failed to verify certificate") ||
		strings.Contains(lower, "certificate is not trusted")
}

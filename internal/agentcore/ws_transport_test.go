package agentcore

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestClassifyConnectError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		resp     *http.Response
		expected connectErrorKind
	}{
		{
			name:     "nil error returns none",
			err:      nil,
			resp:     nil,
			expected: errKindNone,
		},
		{
			name: "401 returns auth",
			err:  errors.New("websocket: bad handshake"),
			resp: &http.Response{
				StatusCode: http.StatusUnauthorized,
			},
			expected: errKindAuth,
		},
		{
			name: "403 returns auth",
			err:  errors.New("websocket: bad handshake"),
			resp: &http.Response{
				StatusCode: http.StatusForbidden,
			},
			expected: errKindAuth,
		},
		{
			name:     "connection refused returns transient",
			err:      errors.New("dial tcp 127.0.0.1:8080: connect: connection refused"),
			resp:     nil,
			expected: errKindTransient,
		},
		{
			name:     "DNS failure returns transient",
			err:      errors.New("dial tcp: lookup hub.example.com: no such host"),
			resp:     nil,
			expected: errKindTransient,
		},
		{
			name: "500 returns transient",
			err:  errors.New("websocket: bad handshake"),
			resp: &http.Response{
				StatusCode: http.StatusInternalServerError,
			},
			expected: errKindTransient,
		},
		{
			name:     "timeout returns transient",
			err:      errors.New("dial tcp 127.0.0.1:8080: i/o timeout"),
			resp:     nil,
			expected: errKindTransient,
		},
		{
			name:     "unknown error returns transient",
			err:      errors.New("something unexpected"),
			resp:     nil,
			expected: errKindTransient,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyConnectError(tc.err, tc.resp)
			if got != tc.expected {
				t.Errorf("classifyConnectError(%v, %v) = %s, want %s", tc.err, tc.resp, got, tc.expected)
			}
		})
	}
}

func TestConnectErrorKindString(t *testing.T) {
	tests := []struct {
		kind     connectErrorKind
		expected string
	}{
		{errKindNone, "none"},
		{errKindAuth, "auth_failed"},
		{errKindTransient, "transient"},
		{connectErrorKind(99), "unknown"},
	}
	for _, tc := range tests {
		if got := tc.kind.String(); got != tc.expected {
			t.Errorf("connectErrorKind(%d).String() = %q, want %q", tc.kind, got, tc.expected)
		}
	}
}

func TestConnectionStateMethod(t *testing.T) {
	tests := []struct {
		name              string
		connected         bool
		lastError         string
		disconnectedAt    time.Time
		expectedState     string
		expectedLastErr   string
		expectedDiscoTime time.Time
	}{
		{
			name:            "connected returns connected state",
			connected:       true,
			lastError:       "",
			expectedState:   "connected",
			expectedLastErr: "",
		},
		{
			name:            "auth failure returns auth_failed state",
			connected:       false,
			lastError:       "auth_failed",
			disconnectedAt:  time.Date(2026, 2, 22, 10, 0, 0, 0, time.UTC),
			expectedState:   "auth_failed",
			expectedLastErr: "auth_failed",
		},
		{
			name:            "transient error returns connecting state",
			connected:       false,
			lastError:       "transient",
			disconnectedAt:  time.Date(2026, 2, 22, 10, 0, 0, 0, time.UTC),
			expectedState:   "connecting",
			expectedLastErr: "transient",
		},
		{
			name:            "no error and not connected returns disconnected",
			connected:       false,
			lastError:       "",
			expectedState:   "disconnected",
			expectedLastErr: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tr := &wsTransport{
				connected:      tc.connected,
				lastError:      tc.lastError,
				disconnectedAt: tc.disconnectedAt,
			}

			state, lastErr, discoAt := tr.ConnectionState()

			if state != tc.expectedState {
				t.Errorf("ConnectionState() state = %q, want %q", state, tc.expectedState)
			}
			if lastErr != tc.expectedLastErr {
				t.Errorf("ConnectionState() lastErr = %q, want %q", lastErr, tc.expectedLastErr)
			}
			if tc.name == "auth failure returns auth_failed state" {
				if !discoAt.Equal(tc.disconnectedAt) {
					t.Errorf("ConnectionState() disconnectedAt = %v, want %v", discoAt, tc.disconnectedAt)
				}
			}
		})
	}
}

func TestTransportUpdateToken(t *testing.T) {
	t.Parallel()
	transport := &wsTransport{
		token: "old-token",
	}
	transport.consecutiveAuthFailures = 5
	transport.lastError = "auth_failed"
	transport.updateToken("new-token")

	transport.mu.Lock()
	defer transport.mu.Unlock()
	if transport.token != "new-token" {
		t.Fatalf("expected token %q, got %q", "new-token", transport.token)
	}
	if transport.consecutiveAuthFailures != 0 {
		t.Fatalf("expected auth failures reset to 0, got %d", transport.consecutiveAuthFailures)
	}
	if transport.lastError != "" {
		t.Fatalf("expected lastError cleared, got %q", transport.lastError)
	}
}

func TestMarkDisconnectedStopsPing(t *testing.T) {
	t.Parallel()
	transport := &wsTransport{}
	transport.connected = true
	transport.pingDone = make(chan struct{})
	transport.markDisconnected()
	if transport.Connected() {
		t.Fatal("expected disconnected after markDisconnected")
	}
	if transport.pingDone != nil {
		t.Fatal("expected pingDone to be nil after markDisconnected")
	}
}

func TestTransportStatsInitialValues(t *testing.T) {
	t.Parallel()
	before := time.Now()
	transport := newWSTransport("ws://hub.example.com/ws", "token", "node-01", "linux", "v1.2.3", nil, "", nil)
	after := time.Now()

	sent, received, reconnects, uptime := transport.Stats()
	if sent != 0 {
		t.Errorf("initial messagesSent = %d, want 0", sent)
	}
	if received != 0 {
		t.Errorf("initial messagesReceived = %d, want 0", received)
	}
	if reconnects != 0 {
		t.Errorf("initial reconnectCount = %d, want 0", reconnects)
	}
	if uptime < 0 {
		t.Errorf("uptime = %v, want >= 0", uptime)
	}
	if transport.startedAt.Before(before) || transport.startedAt.After(after) {
		t.Errorf("startedAt = %v, expected between %v and %v", transport.startedAt, before, after)
	}
}

func TestTransportStatsCounterIncrements(t *testing.T) {
	t.Parallel()
	transport := &wsTransport{startedAt: time.Now()}

	atomic.AddInt64(&transport.messagesSent, 5)
	atomic.AddInt64(&transport.messagesReceived, 3)
	atomic.AddInt64(&transport.reconnectCount, 2)

	sent, received, reconnects, uptime := transport.Stats()
	if sent != 5 {
		t.Errorf("messagesSent = %d, want 5", sent)
	}
	if received != 3 {
		t.Errorf("messagesReceived = %d, want 3", received)
	}
	if reconnects != 2 {
		t.Errorf("reconnectCount = %d, want 2", reconnects)
	}
	if uptime < 0 {
		t.Errorf("uptime = %v, want >= 0", uptime)
	}
}

func TestValidateWebSocketTransportURLRequiresSecureSchemeByDefault(t *testing.T) {
	t.Setenv(envAllowInsecureTransport, "false")
	if err := validateWebSocketTransportURL("ws://hub.example.com/ws/agent"); err == nil {
		t.Fatalf("expected ws URL to be rejected without insecure opt-in")
	}
	if err := validateWebSocketTransportURL("wss://hub.example.com/ws/agent"); err != nil {
		t.Fatalf("expected wss URL to be accepted, got %v", err)
	}
}

func TestValidateWebSocketTransportURLAllowsInsecureWhenOptedIn(t *testing.T) {
	t.Setenv(envAllowInsecureTransport, "true")
	if err := validateWebSocketTransportURL("ws://hub.example.com/ws/agent"); err != nil {
		t.Fatalf("expected ws URL to be allowed with explicit opt-in, got %v", err)
	}
}

func TestConnectWithResponseSendsBearerTokenHeaders(t *testing.T) {
	t.Setenv(envAllowInsecureTransport, "true")

	headersSeen := make(chan http.Header, 1)
	done := make(chan struct{})
	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headersSeen <- r.Header.Clone()
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade failed: %v", err)
			return
		}
		defer conn.Close()
		<-done
	}))
	defer func() {
		close(done)
		server.Close()
	}()

	transport := newWSTransport("ws"+server.URL[len("http"):], "token-123", "node-01", "linux", "v1.2.3", nil, "", nil)
	defer transport.Close()

	resp, err := transport.connectWithResponse(context.Background())
	if err != nil {
		t.Fatalf("connectWithResponse returned error: %v", err)
	}
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if !transport.Connected() {
		t.Fatalf("expected transport to be marked connected")
	}

	select {
	case headers := <-headersSeen:
		if got := headers.Get("Authorization"); got != "Bearer token-123" {
			t.Fatalf("expected Authorization header, got %q", got)
		}
		if got := headers.Get("X-Asset-ID"); got != "node-01" {
			t.Fatalf("expected X-Asset-ID header, got %q", got)
		}
		if got := headers.Get("X-Platform"); got != "linux" {
			t.Fatalf("expected X-Platform header, got %q", got)
		}
		if got := headers.Get("X-Agent-Version"); got != "v1.2.3" {
			t.Fatalf("expected X-Agent-Version header, got %q", got)
		}
		if got := headers.Get("X-Request-Enrollment"); got != "" {
			t.Fatalf("expected no enrollment header when token is present, got %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for websocket request headers")
	}
}

func TestConnectWithResponseRequestsEnrollmentWhenTokenEmpty(t *testing.T) {
	t.Setenv(envAllowInsecureTransport, "true")

	headersSeen := make(chan http.Header, 1)
	done := make(chan struct{})
	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headersSeen <- r.Header.Clone()
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade failed: %v", err)
			return
		}
		defer conn.Close()
		<-done
	}))
	defer func() {
		close(done)
		server.Close()
	}()

	transport := newWSTransport(
		"ws"+server.URL[len("http"):],
		"",
		"pending-node-01",
		"linux",
		"v1.2.3",
		nil,
		"",
		&deviceIdentity{
			KeyAlgorithm:    "ed25519",
			PublicKeyBase64: "device-public-key",
			Fingerprint:     "device-fingerprint",
		},
	)
	defer transport.Close()

	resp, err := transport.connectWithResponse(context.Background())
	if err != nil {
		t.Fatalf("connectWithResponse returned error: %v", err)
	}
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}

	select {
	case headers := <-headersSeen:
		if got := headers.Get("Authorization"); got != "" {
			t.Fatalf("expected no Authorization header in enrollment mode, got %q", got)
		}
		if got := headers.Get("X-Request-Enrollment"); got != "true" {
			t.Fatalf("expected enrollment header, got %q", got)
		}
		if got := headers.Get("X-Asset-ID"); got != "pending-node-01" {
			t.Fatalf("expected X-Asset-ID header, got %q", got)
		}
		if got := headers.Get("X-Device-Key-Alg"); got != "ed25519" {
			t.Fatalf("expected X-Device-Key-Alg header, got %q", got)
		}
		if got := headers.Get("X-Device-Public-Key"); got != "device-public-key" {
			t.Fatalf("expected X-Device-Public-Key header, got %q", got)
		}
		if got := headers.Get("X-Device-Fingerprint"); got != "device-fingerprint" {
			t.Fatalf("expected X-Device-Fingerprint header, got %q", got)
		}
		if got := headers.Get("X-Hostname"); got == "" {
			t.Fatalf("expected X-Hostname header to be set")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for websocket request headers")
	}
}

func TestReconnectLoopReEnrollsAfterAuthFailureThreshold(t *testing.T) {
	waits := make(chan time.Duration, 4)
	transport := &wsTransport{
		token: "stale-token",
		timeAfter: func(d time.Duration) <-chan time.Time {
			waits <- d
			ch := make(chan time.Time, 1)
			ch <- time.Now()
			return ch
		},
		now:    func() time.Time { return time.Date(2026, 3, 8, 12, 0, 0, 0, time.UTC) },
		jitter: func(time.Duration) time.Duration { return 0 },
	}

	connectCalls := 0
	transport.connectWithResponseFn = func(context.Context) (*http.Response, error) {
		connectCalls++
		if connectCalls <= authFailureThreshold {
			return &http.Response{StatusCode: http.StatusUnauthorized}, errors.New("websocket: bad handshake")
		}
		transport.mu.Lock()
		transport.connected = true
		transport.consecutiveAuthFailures = 0
		transport.lastError = ""
		transport.mu.Unlock()
		return nil, nil
	}

	reEnrollCalls := 0
	transport.reEnrollFn = func() (string, error) {
		reEnrollCalls++
		return "fresh-token", nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		transport.reconnectLoop(ctx, func() {
			transport.markDisconnected()
			cancel()
		})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for reconnect loop to exit")
	}

	if reEnrollCalls != 1 {
		t.Fatalf("expected one re-enrollment attempt, got %d", reEnrollCalls)
	}
	if connectCalls != authFailureThreshold+1 {
		t.Fatalf("expected %d connect attempts, got %d", authFailureThreshold+1, connectCalls)
	}
	if transport.token != "fresh-token" {
		t.Fatalf("expected token to be refreshed, got %q", transport.token)
	}
	if got := atomic.LoadInt64(&transport.reconnectCount); got != 1 {
		t.Fatalf("expected reconnect count 1, got %d", got)
	}

	gotWaits := []time.Duration{<-waits, <-waits}
	wantWaits := []time.Duration{time.Second, 2 * time.Second}
	for i := range wantWaits {
		if gotWaits[i] != wantWaits[i] {
			t.Fatalf("wait[%d] = %s, want %s", i, gotWaits[i], wantWaits[i])
		}
	}
	select {
	case extra := <-waits:
		t.Fatalf("unexpected extra wait duration %s", extra)
	default:
	}
}

func TestReconnectLoopForcesReconnectOnNetworkChange(t *testing.T) {
	networkChanged := make(chan struct{}, 1)
	transport := &wsTransport{
		networkChanged: networkChanged,
	}

	connectCalls := 0
	transport.connectWithResponseFn = func(context.Context) (*http.Response, error) {
		connectCalls++
		transport.mu.Lock()
		transport.connected = true
		transport.mu.Unlock()
		return nil, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	var onConnectCalls int32
	go func() {
		transport.reconnectLoop(ctx, func() {
			switch atomic.AddInt32(&onConnectCalls, 1) {
			case 1:
				networkChanged <- struct{}{}
			case 2:
				transport.markDisconnected()
				cancel()
			}
		})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for reconnect loop to exit")
	}

	if connectCalls != 2 {
		t.Fatalf("expected 2 connect attempts, got %d", connectCalls)
	}
	if got := atomic.LoadInt32(&onConnectCalls); got != 2 {
		t.Fatalf("expected onConnect to fire twice, got %d", got)
	}
	if got := atomic.LoadInt64(&transport.reconnectCount); got != 2 {
		t.Fatalf("expected reconnect count 2, got %d", got)
	}
}

package portainer

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labtether/labtether/internal/securityruntime"
)

func TestDialPortainerExecSocketUsesBearerAuthorization(t *testing.T) {
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOWLIST_MODE", "false")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_PRIVATE", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")

	const token = "portainer-exec-jwt"
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+token {
			http.Error(w, "missing bearer", http.StatusUnauthorized)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		_ = conn.Close()
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/websocket/exec?endpointId=1&id=abc"
	conn, err := dialPortainerExecSocket(wsURL, token, false)
	if err != nil {
		t.Fatalf("dialPortainerExecSocket: %v", err)
	}
	_ = conn.Close()
}

func TestPortainerExecJWTIsRedactedFromURLDialErrorAndLogs(t *testing.T) {
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOWLIST_MODE", "false")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_PRIVATE", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")

	const token = "sensitive-portainer-exec-jwt"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.String(), token) {
			t.Errorf("JWT leaked into request URL: %s", r.URL.String())
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+token {
			t.Errorf("Authorization = %q", got)
		}
		http.Error(w, "upstream reflected "+token, http.StatusUnauthorized)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/websocket/exec?endpointId=1&id=abc"
	_, err := dialPortainerExecSocket(wsURL, token, false)
	if err == nil {
		t.Fatal("expected rejected websocket handshake")
	}
	if strings.Contains(err.Error(), token) || !strings.Contains(err.Error(), "[REDACTED]") {
		t.Fatalf("dial error was not safely redacted: %v", err)
	}

	var logs bytes.Buffer
	originalWriter := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(originalWriter)
	securityruntime.Logf("portainer-exec: dial failed: %v", err)
	if strings.Contains(logs.String(), token) {
		t.Fatalf("JWT leaked into log output: %s", logs.String())
	}
}

func TestDialPortainerExecSocketRejectsCredentialBearingURLs(t *testing.T) {
	const token = "sensitive-portainer-exec-jwt"
	for _, wsURL := range []string{
		"ws://" + token + "@127.0.0.1/api/websocket/exec",
		"ws://127.0.0.1/api/websocket/exec?token=" + url.QueryEscape(token),
		"ws://127.0.0.1/" + url.PathEscape(token) + "/api/websocket/exec",
		"ws://127.0.0.1/api/websocket/exec#" + url.QueryEscape(token),
	} {
		_, err := dialPortainerExecSocket(wsURL, token, false)
		if err == nil || strings.Contains(err.Error(), token) {
			t.Fatalf("credential-bearing URL was not safely rejected: url=%q err=%v", wsURL, err)
		}
	}
}

func TestBridgePortainerExecDoesNotForwardUpstreamCloseSecret(t *testing.T) {
	const token = "sensitive-portainer-close-jwt"
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "reflected "+token),
			time.Now().Add(time.Second),
		)
	}))
	defer upstream.Close()

	upstreamURL := "ws" + strings.TrimPrefix(upstream.URL, "http")
	upstreamConn, _, err := websocket.DefaultDialer.Dial(upstreamURL, nil)
	if err != nil {
		t.Fatalf("dial upstream websocket: %v", err)
	}

	deps := &Deps{
		StartBrowserWebSocketKeepalive: func(*websocket.Conn, *sync.Mutex, string) func() {
			return func() {}
		},
		TouchBrowserWebSocketReadDeadline: func(*websocket.Conn) error { return nil },
	}
	browserServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		browserConn, upgradeErr := upgrader.Upgrade(w, r, nil)
		if upgradeErr != nil {
			return
		}
		defer browserConn.Close()
		deps.bridgePortainerExec(browserConn, upstreamConn)
	}))
	defer browserServer.Close()

	browserURL := "ws" + strings.TrimPrefix(browserServer.URL, "http")
	browserConn, _, err := websocket.DefaultDialer.Dial(browserURL, nil)
	if err != nil {
		t.Fatalf("dial browser websocket: %v", err)
	}
	defer browserConn.Close()
	_ = browserConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, _, err = browserConn.ReadMessage()
	if err == nil {
		t.Fatal("expected browser bridge to close")
	}
	if strings.Contains(err.Error(), token) {
		t.Fatalf("upstream close secret reached browser close text: %v", err)
	}
}

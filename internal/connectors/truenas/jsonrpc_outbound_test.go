package truenas

import (
	"context"
	"errors"
	"github.com/gorilla/websocket"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDialTrueNASWebSocketRejectsDisallowedEndpointBeforeNetworkDial(t *testing.T) {
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "false")
	t.Setenv("LABTETHER_OUTBOUND_ALLOWLIST_MODE", "false")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "false")

	conn, err := dialTrueNASWebSocket(context.Background(), "wss://127.0.0.1:443/websocket", false, maxWebSocketMessageBytes)
	if err == nil {
		if conn != nil {
			_ = conn.Close()
		}
		t.Fatal("expected outbound policy to reject a loopback endpoint")
	}
	if !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("dial rejection = %v, want loopback policy error", err)
	}
}

func TestDialTrueNASWebSocketEnforcesFragmentedResponseLimit(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)

	serverErr := make(chan error, 1)
	upgrader := websocket.Upgrader{
		CheckOrigin:     func(_ *http.Request) bool { return true },
		WriteBufferSize: 32,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			serverErr <- err
			return
		}
		defer conn.Close()

		writer, err := conn.NextWriter(websocket.TextMessage)
		if err == nil {
			payload := []byte(`"` + strings.Repeat("x", 200) + `"`)
			for len(payload) > 0 && err == nil {
				chunkSize := 40
				if len(payload) < chunkSize {
					chunkSize = len(payload)
				}
				_, err = writer.Write(payload[:chunkSize])
				payload = payload[chunkSize:]
			}
			if closeErr := writer.Close(); err == nil {
				err = closeErr
			}
		}
		serverErr <- err
	}))
	defer server.Close()

	const testLimit = int64(128)
	conn, err := dialTrueNASWebSocket(context.Background(), serverURLToWS(server.URL), false, testLimit)
	if err != nil {
		t.Fatalf("dialTrueNASWebSocket() error = %v", err)
	}
	defer conn.Close()

	var decoded string
	if err := conn.ReadJSON(&decoded); !errors.Is(err, websocket.ErrReadLimit) {
		t.Fatalf("ReadJSON() error = %v, want ErrReadLimit", err)
	}
	select {
	case err := <-serverErr:
		if err != nil {
			t.Fatalf("write fragmented response: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not finish fragmented response")
	}
}

func TestJSONRPCCallRejectsDisallowedOutboundEndpoint(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	t.Setenv("LABTETHER_OUTBOUND_ALLOWLIST_MODE", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_PRIVATE", "false")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "false")
	t.Setenv("LABTETHER_OUTBOUND_ALLOWED_HOSTS", "allowed.example.com")

	dialCalled := false
	stubDialWSForTest(t, func(ctx context.Context, endpoint string, skipVerify bool) (wsConn, error) {
		dialCalled = true
		return nil, errors.New("dial should not be called for disallowed endpoints")
	})

	client := &Client{
		BaseURL: "wss://blocked.example.net",
		APIKey:  "test-api-key",
		Timeout: 5 * time.Second,
	}

	var result map[string]any
	err := client.Call(context.Background(), "system.info", nil, &result)
	if err == nil {
		t.Fatal("expected outbound policy validation error")
	}
	if !strings.Contains(err.Error(), "endpoint validation") || !strings.Contains(err.Error(), "not allowlisted") {
		t.Fatalf("unexpected error: %v", err)
	}
	if dialCalled {
		t.Fatal("expected dialWS not to be called on endpoint validation failure")
	}
}

func TestJSONRPCSubscribeRejectsDisallowedOutboundEndpoint(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	t.Setenv("LABTETHER_OUTBOUND_ALLOWLIST_MODE", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_PRIVATE", "false")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "false")
	t.Setenv("LABTETHER_OUTBOUND_ALLOWED_HOSTS", "allowed.example.com")

	dialCalled := false
	stubDialWSForTest(t, func(ctx context.Context, endpoint string, skipVerify bool) (wsConn, error) {
		dialCalled = true
		return nil, errors.New("dial should not be called for disallowed endpoints")
	})

	client := &Client{
		BaseURL: "wss://blocked.example.net",
		APIKey:  "test-api-key",
		Timeout: 5 * time.Second,
	}

	err := client.Subscribe(context.Background(), "alert.list", func(event SubscriptionEvent) error { return nil })
	if err == nil {
		t.Fatal("expected outbound policy validation error")
	}
	if !strings.Contains(err.Error(), "endpoint validation") || !strings.Contains(err.Error(), "not allowlisted") {
		t.Fatalf("unexpected error: %v", err)
	}
	if dialCalled {
		t.Fatal("expected dialWS not to be called on endpoint validation failure")
	}
}

func TestClientWSEndpointVariants(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	cases := []struct {
		name string
		base string
		want string
	}{
		{name: "empty default", base: "", want: "wss://localhost/api/current"},
		{name: "https", base: "https://truenas.local", want: "wss://truenas.local/api/current"},
		{name: "http with path query fragment", base: "http://truenas.local:8080/old/path?x=1#frag", want: "ws://truenas.local:8080/api/current"},
		{name: "ws passthrough", base: "ws://truenas.local/custom", want: "ws://truenas.local/api/current"},
		{name: "wss passthrough", base: "wss://truenas.local/custom", want: "wss://truenas.local/api/current"},
		{name: "unknown scheme coerced", base: "ftp://truenas.local/root", want: "wss://truenas.local/api/current"},
		{name: "parse fallback", base: "http://%", want: "wss://%/api/current"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := (&Client{BaseURL: tc.base}).wsEndpoint()
			if got != tc.want {
				t.Fatalf("wsEndpoint(%q) = %q, want %q", tc.base, got, tc.want)
			}
		})
	}
}

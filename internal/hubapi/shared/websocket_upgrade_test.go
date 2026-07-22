package shared

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func TestUpgradeWebSocketRejectsMissingUpgraderOrOriginPolicy(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "http://example.test/ws", nil)

	if _, err := UpgradeWebSocket(nil, w, r, nil); !errors.Is(err, ErrWebSocketUpgraderRequired) {
		t.Fatalf("UpgradeWebSocket(nil) error = %v, want %v", err, ErrWebSocketUpgraderRequired)
	}
	if _, err := UpgradeWebSocket(&websocket.Upgrader{}, w, r, nil); !errors.Is(err, ErrWebSocketOriginPolicyRequired) {
		t.Fatalf("UpgradeWebSocket without CheckOrigin error = %v, want %v", err, ErrWebSocketOriginPolicyRequired)
	}
}

func TestUpgradeWebSocketAcceptsExplicitOriginPolicy(t *testing.T) {
	t.Parallel()

	policyCalled := make(chan struct{}, 1)
	upgradeResult := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{
			CheckOrigin: func(_ *http.Request) bool {
				policyCalled <- struct{}{}
				return true
			},
		}
		conn, err := UpgradeWebSocket(&upgrader, w, r, nil)
		upgradeResult <- err
		if conn != nil {
			_ = conn.Close()
		}
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	_ = conn.Close()

	if err := <-upgradeResult; err != nil {
		t.Fatalf("UpgradeWebSocket() error = %v", err)
	}
	select {
	case <-policyCalled:
	default:
		t.Fatal("explicit origin policy was not called")
	}
}

package truenas

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMutationCanceledDuringAuthenticationDoesNotDispatch(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	methodSeen := make(chan bool, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer conn.Close()
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
		var auth rpcCall
		if err := conn.ReadJSON(&auth); err != nil {
			t.Errorf("read auth: %v", err)
			return
		}
		if auth.Method != "auth.login_with_api_key" {
			t.Errorf("unexpected first call: %s", auth.Method)
			return
		}
		// Cancellation is complete before the valid auth reply reaches Call.
		cancel()
		if err := writeRPCResult(conn, auth.ID, true); err != nil {
			t.Errorf("auth reply: %v", err)
			return
		}
		var method rpcCall
		methodSeen <- conn.ReadJSON(&method) == nil
	}))
	defer server.Close()
	client := &Client{BaseURL: server.URL, APIKey: "fixture-key", Timeout: 2 * time.Second}
	err := client.CallMutation(ctx, "service.restart", []any{"nfs"}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled request result: %v", err)
	}
	select {
	case seen := <-methodSeen:
		if seen {
			t.Fatal("canceled mutation dispatched after authentication")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("owned socket did not close")
	}
}

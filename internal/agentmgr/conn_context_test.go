package agentmgr

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// The first Err observation marks entry before waiting for the socket mutex.
// Capturing its value before signaling makes cancellation at that gate exact.
type observedSendContext struct {
	context.Context
	observed chan struct{}
	once     sync.Once
}

func (c *observedSendContext) Err() error {
	err := c.Context.Err()
	c.once.Do(func() { close(c.observed) })
	return err
}

func TestSendContextCanceledAtOwnershipGateDoesNotWrite(t *testing.T) {
	for _, gate := range []string{"mutex", "credential"} {
		t.Run(gate, func(t *testing.T) {
			received := make(chan Message, 2)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				socket, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
				if err != nil {
					return
				}
				defer socket.Close()
				for {
					var msg Message
					if socket.ReadJSON(&msg) != nil {
						return
					}
					received <- msg
				}
			}))
			socket, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
			if err != nil {
				server.Close()
				t.Fatal(err)
			}
			conn := NewAgentConn(socket, "context-gate", "linux")
			defer server.Close()
			defer conn.Close()
			base, cancel := context.WithCancel(context.Background())
			defer cancel()
			ctx := &observedSendContext{Context: base, observed: make(chan struct{})}
			entered := ctx.observed
			var release func()
			var once sync.Once
			if gate == "mutex" {
				conn.mu.Lock()
				release = func() { once.Do(conn.mu.Unlock) }
			} else {
				started := make(chan struct{})
				unblock := make(chan struct{})
				entered = started
				conn.SetCredentialValidator(func() error { close(started); <-unblock; return nil })
				release = func() { once.Do(func() { close(unblock) }) }
			}
			defer release()
			finished := make(chan error, 1)
			done := make(chan struct{})
			t.Cleanup(func() {
				release()
				cancel()
				conn.Close()
				select {
				case <-done:
				case <-time.After(2 * time.Second):
					t.Error("sender did not exit during fixture cleanup")
				}
			})
			go func() {
				defer close(done)
				finished <- conn.SendContext(ctx, Message{Type: MsgCommandRequest, ID: "must-not-send"})
			}()
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("sender did not reach ownership gate")
			}
			cancel()
			release()
			select {
			case err := <-finished:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("send error: %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("canceled send did not return after gate release")
			}
			conn.SetCredentialValidator(nil)
			if err := conn.Send(Message{Type: MsgConfigUpdate, ID: "live-socket"}); err != nil {
				t.Fatal(err)
			}
			select {
			case msg := <-received:
				if msg.ID != "live-socket" {
					t.Fatalf("canceled send reached peer: %+v", msg)
				}
			case <-time.After(time.Second):
				t.Fatal("shared socket was not usable after cancellation")
			}
		})
	}
}

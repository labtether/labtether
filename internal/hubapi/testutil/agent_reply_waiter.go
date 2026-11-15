package testutil

import (
	"context"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
)

// AwaitReply registers a waiter for requestID in p, as a request to device
// would, for tests that call a reply handler directly. Each reply the
// handler hands to the waiter is copied to the returned channel before
// Deliver returns, so a test can check at once, without a timer, whether a
// reply was taken. The channel holds one reply and drops later ones, as the
// one-slot bridge channels did. The waiter is removed when the test ends.
func AwaitReply[T any](t testing.TB, p *agentmgr.Pending[T], requestID, device string) <-chan T {
	t.Helper()
	taken := make(chan T, 1)
	sent := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = agentmgr.Request(ctx, p, agentmgr.Call[struct{}, T]{
			Type:    "test.await_reply",
			ID:      requestID,
			Send:    func(agentmgr.Message) error { close(sent); return nil },
			Device:  device,
			Timeout: time.Minute,
			Accept: func(reply T) bool {
				select {
				case taken <- reply:
				default:
				}
				return true
			},
		})
	}()
	t.Cleanup(func() { cancel(); <-done })
	select {
	case <-sent:
	case <-done:
		t.Fatalf("waiter for %q was not registered", requestID)
	case <-time.After(2 * time.Second):
		t.Fatalf("waiter for %q was not registered within 2s", requestID)
	}
	return taken
}

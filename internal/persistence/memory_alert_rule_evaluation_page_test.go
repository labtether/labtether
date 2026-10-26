package persistence

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type observedRulePageContext struct {
	context.Context
	firstCheck chan struct{}
	once       sync.Once
}

func (c *observedRulePageContext) Err() error {
	err := c.Context.Err()
	c.once.Do(func() { close(c.firstCheck) })
	return err
}

// A read blocked behind a real writer must return when its caller cancels;
// merely checking the context before an ordinary RLock does not satisfy this.
func TestMemoryAlertRulePageCancelsWhileWriterOwnsLock(t *testing.T) {
	store := NewMemoryAlertStore()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	observed := &observedRulePageContext{Context: ctx, firstCheck: make(chan struct{})}
	store.mu.Lock()
	done := make(chan error, 1)
	go func() {
		_, err := store.PageActiveAlertRules(observed, AlertRuleEvaluationCursor{}, 128)
		done <- err
	}()
	// Err captures nil before announcing this boundary, so cancellation cannot
	// accidentally make an old pre-lock-only check pass the regression.
	<-observed.firstCheck
	cancel()
	select {
	case err := <-done:
		store.mu.Unlock()
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("blocked read ignored cancellation: %v", err)
		}
	case <-time.After(time.Second):
		store.mu.Unlock()
		<-done // Release/join the exact reader before failing the check.
		t.Fatal("canceled read waited for the writer")
	}
}

package main

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/persistence"
)

type runtimeLeasePingerStub struct {
	mu       sync.Mutex
	pingErr  error
	pinged   chan struct{}
	pingOnce sync.Once
}

type cancelDuringPingLeasePinger struct {
	cancel   context.CancelFunc
	pinged   chan struct{}
	pingOnce sync.Once
}

func (s *cancelDuringPingLeasePinger) Ping(context.Context) error {
	s.pingOnce.Do(func() { close(s.pinged) })
	s.cancel()
	return persistence.ErrHubRuntimeLeaseLost
}

func (s *runtimeLeasePingerStub) Ping(context.Context) error {
	s.mu.Lock()
	err := s.pingErr
	s.mu.Unlock()
	s.pingOnce.Do(func() { close(s.pinged) })
	return err
}

func TestMonitorHubRuntimeLeaseCancelsOnLoss(t *testing.T) {
	parent, stop := context.WithCancel(context.Background())
	defer stop()
	ctx, cancel := context.WithCancelCause(parent)
	lease := &runtimeLeasePingerStub{
		pingErr: persistence.ErrHubRuntimeLeaseLost,
		pinged:  make(chan struct{}),
	}
	go monitorHubRuntimeLease(ctx, lease, time.Millisecond, time.Second, cancel)

	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("runtime context was not canceled after lease loss")
	}
	if !errors.Is(context.Cause(ctx), persistence.ErrHubRuntimeLeaseLost) {
		t.Fatalf("cancel cause = %v, want ErrHubRuntimeLeaseLost", context.Cause(ctx))
	}
}

func TestMonitorHubRuntimeLeaseStopsWithParent(t *testing.T) {
	parent, stop := context.WithCancel(context.Background())
	ctx, cancel := context.WithCancelCause(parent)
	lease := &runtimeLeasePingerStub{pinged: make(chan struct{})}
	done := make(chan struct{})
	go func() {
		defer close(done)
		monitorHubRuntimeLease(ctx, lease, time.Millisecond, time.Second, cancel)
	}()

	select {
	case <-lease.pinged:
	case <-time.After(time.Second):
		t.Fatal("lease monitor did not ping")
	}
	stop()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("lease monitor did not stop with parent context")
	}
}

func TestMonitorHubRuntimeLeaseIgnoresPingErrorAfterIntentionalCancellation(t *testing.T) {
	ctx, stop := context.WithCancel(context.Background())
	lease := &cancelDuringPingLeasePinger{
		cancel: stop,
		pinged: make(chan struct{}),
	}
	cancelCalled := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		monitorHubRuntimeLease(ctx, lease, time.Millisecond, time.Second, func(err error) {
			cancelCalled <- err
		})
	}()

	select {
	case <-lease.pinged:
	case <-time.After(time.Second):
		t.Fatal("lease monitor did not enter Ping")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("lease monitor did not exit after intentional cancellation")
	}
	select {
	case err := <-cancelCalled:
		t.Fatalf("intentional cancellation was relabeled as lease loss: %v", err)
	default:
	}
}

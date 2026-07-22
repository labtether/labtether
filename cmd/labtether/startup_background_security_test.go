package main

import (
	"context"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/persistence"
)

func TestRequestAuditWorkerDrainsQueueAndTerminatesOnCancel(t *testing.T) {
	store := persistence.NewMemoryAuditStore()
	srv := &apiServer{
		auditStore:   store,
		auditEventCh: make(chan audit.Event, 2),
	}
	srv.auditEventCh <- audit.Event{ID: "audit-shutdown", Type: "api.request", Timestamp: time.Now().UTC()}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runRequestAuditWorker(ctx, srv)
		close(done)
	}()
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("request-audit worker did not terminate after cancellation")
	}

	events, err := store.List(10, 0)
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	if len(events) != 1 || events[0].ID != "audit-shutdown" {
		t.Fatalf("expected accepted audit event to drain during shutdown, got %#v", events)
	}
}

package persistence

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func TestPostgresHubRuntimeLeaseIsExclusiveAndReleasable(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("DATABASE_URL not set, skipping PostgreSQL runtime-lease integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	firstStore, err := NewPostgresStore(ctx, databaseURL)
	if err != nil {
		t.Fatalf("create first store: %v", err)
	}
	defer firstStore.Close()
	secondStore, err := NewPostgresStore(ctx, databaseURL)
	if err != nil {
		t.Fatalf("create second store: %v", err)
	}
	defer secondStore.Close()

	first, err := firstStore.AcquireHubRuntimeLease(ctx)
	if err != nil {
		t.Fatalf("acquire first lease: %v", err)
	}
	if err := first.Ping(ctx); err != nil {
		t.Fatalf("ping first lease: %v", err)
	}
	if _, err := secondStore.AcquireHubRuntimeLease(ctx); !errors.Is(err, ErrHubRuntimeLeaseHeld) {
		t.Fatalf("second lease error = %v, want ErrHubRuntimeLeaseHeld", err)
	}
	if err := first.Release(ctx); err != nil {
		t.Fatalf("release first lease: %v", err)
	}
	if err := first.Release(ctx); err != nil {
		t.Fatalf("second release must be idempotent: %v", err)
	}

	second, err := secondStore.AcquireHubRuntimeLease(ctx)
	if err != nil {
		t.Fatalf("acquire lease after release: %v", err)
	}
	if err := second.Release(ctx); err != nil {
		t.Fatalf("release second lease: %v", err)
	}
}

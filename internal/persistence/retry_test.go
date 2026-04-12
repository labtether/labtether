// internal/persistence/retry_test.go
package persistence

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
)

func TestRetryDB_SucceedsFirst(t *testing.T) {
	var calls atomic.Int32
	err := RetryDB(context.Background(), 3, func(ctx context.Context) error {
		calls.Add(1)
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("expected 1 call, got %d", calls.Load())
	}
}

func TestRetryDB_RetriesOnError(t *testing.T) {
	var calls atomic.Int32
	err := RetryDB(context.Background(), 3, func(ctx context.Context) error {
		n := calls.Add(1)
		if n < 3 {
			return errors.New("transient")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil after retries, got %v", err)
	}
	if calls.Load() != 3 {
		t.Fatalf("expected 3 calls, got %d", calls.Load())
	}
}

func TestRetryDB_ExhaustsRetries(t *testing.T) {
	var calls atomic.Int32
	err := RetryDB(context.Background(), 3, func(ctx context.Context) error {
		calls.Add(1)
		return errors.New("persistent")
	})
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if calls.Load() != 3 {
		t.Fatalf("expected 3 calls, got %d", calls.Load())
	}
}

func TestRetryDB_RespectsContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := RetryDB(ctx, 3, func(ctx context.Context) error {
		return errors.New("should not retry")
	})
	if err == nil {
		t.Fatal("expected error on cancelled context")
	}
}

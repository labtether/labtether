package servicehttp

import (
	"bytes"
	"context"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRecoverMiddleware_CatchesPanic(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	handler := RecoverMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
	if !strings.Contains(buf.String(), "test panic") {
		t.Errorf("expected panic logged, got: %s", buf.String())
	}
}

func TestRecoverMiddleware_PassesThrough(t *testing.T) {
	handler := RecoverMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestSafeGo_RecoversAndRestarts(t *testing.T) {
	var count atomic.Int32
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	SafeGo(ctx, &wg, "test-loop", func(ctx context.Context) {
		n := count.Add(1)
		if n <= 2 {
			panic("intentional panic")
		}
		<-ctx.Done()
	})

	deadline := time.After(5 * time.Second)
	for {
		if count.Load() >= 3 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for restarts, count=%d", count.Load())
		case <-time.After(100 * time.Millisecond):
		}
	}

	cancel()
	wg.Wait()
}

func TestSafeGo_WaitGroupTracking(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	SafeGo(ctx, &wg, "wg-test", func(ctx context.Context) {
		<-ctx.Done()
	})

	time.Sleep(50 * time.Millisecond)
	cancel()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("WaitGroup never reached zero")
	}
}

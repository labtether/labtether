# Reliability Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers-extended-cc:subagent-driven-development (recommended) or superpowers-extended-cc:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Prevent crashes, hangs, goroutine leaks, and silent failures across the hub and Linux agent.

**Architecture:** Add panic recovery at all goroutine entry points, supervise background loops with restart-on-panic, enforce command kill-on-timeout, add DB connection health checks, and add an agent watchdog for stuck-state detection.

**Tech Stack:** Go stdlib (sync, runtime, context, atomic), pgx pool config, existing codebase patterns.

---

### Task 1: Hub Panic Recovery Middleware and safeGo Helper

**Goal:** Create the foundational `RecoverMiddleware` and `safeGo` helper in the hub's `internal/servicehttp/` package.

**Files:**
- Create: `internal/servicehttp/recover.go`
- Create: `internal/servicehttp/recover_test.go`

**Acceptance Criteria:**
- [ ] RecoverMiddleware catches panics in HTTP handlers, logs stack trace, returns 500
- [ ] safeGo wraps a goroutine with recover, logs panics with goroutine name, restarts after 1s backoff
- [ ] safeGo accepts a sync.WaitGroup and increments/decrements it
- [ ] Tests verify panic recovery and restart behavior

**Verify:** `cd /Users/michael/Development/LabTether/hub && go test ./internal/servicehttp/ -run TestRecover -v`

**Steps:**

- [ ] **Step 1: Write tests for RecoverMiddleware and safeGo**

```go
// internal/servicehttp/recover_test.go
package servicehttp

import (
	"bytes"
	"context"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRecoverMiddleware_CatchesPanic(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(nil)

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
		// Third invocation: block until context done.
		<-ctx.Done()
	})

	// Wait for at least 3 invocations (2 panics + 1 stable).
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

	// Give the goroutine time to start.
	time.Sleep(50 * time.Millisecond)
	cancel()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// success
	case <-time.After(3 * time.Second):
		t.Fatal("WaitGroup never reached zero")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/michael/Development/LabTether/hub && go test ./internal/servicehttp/ -run TestRecover -v 2>&1; go test ./internal/servicehttp/ -run TestSafeGo -v 2>&1`
Expected: Compilation errors — RecoverMiddleware and SafeGo not defined

- [ ] **Step 3: Implement RecoverMiddleware and SafeGo**

```go
// internal/servicehttp/recover.go
package servicehttp

import (
	"context"
	"log"
	"net/http"
	"runtime/debug"
	"sync"
	"time"
)

// RecoverMiddleware wraps an HTTP handler to catch panics. On panic it logs
// the stack trace and returns a 500 response. The server process stays alive.
func RecoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("http: panic recovered on %s %s: %v\n%s", r.Method, r.URL.Path, err, debug.Stack())
				http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// SafeGo launches fn in a goroutine with panic recovery. If fn panics, the
// panic is logged with the goroutine name and fn is restarted after a 1-second
// backoff. The goroutine exits when ctx is cancelled. If wg is non-nil, it is
// used to track the goroutine's lifetime for graceful shutdown.
func SafeGo(ctx context.Context, wg *sync.WaitGroup, name string, fn func(ctx context.Context)) {
	if wg != nil {
		wg.Add(1)
	}
	go func() {
		if wg != nil {
			defer wg.Done()
		}
		for {
			if ctx.Err() != nil {
				return
			}
			func() {
				defer func() {
					if err := recover(); err != nil {
						log.Printf("safego[%s]: panic recovered: %v\n%s", name, err, debug.Stack())
					}
				}()
				fn(ctx)
			}()
			// fn returned or panicked — restart after backoff unless cancelled.
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
		}
	}()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/michael/Development/LabTether/hub && go test ./internal/servicehttp/ -run "TestRecover|TestSafeGo" -v`
Expected: All 4 tests PASS

- [ ] **Step 5: Commit**

```bash
cd /Users/michael/Development/LabTether/hub && git add internal/servicehttp/recover.go internal/servicehttp/recover_test.go && git commit -m "feat: add RecoverMiddleware and SafeGo panic recovery helpers

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: Wire Hub Panic Recovery Into Critical Paths

**Goal:** Apply RecoverMiddleware to all HTTP handlers, add recover() to the WebSocket dispatcher, replace bare goroutines in startup_background.go with SafeGo, and bound the API key touch goroutines.

**Files:**
- Modify: `cmd/labtether/startup_bootstrap.go:296-330`
- Modify: `cmd/labtether/agent_ws_handler.go:407-413`
- Modify: `cmd/labtether/startup_background.go:10-48`
- Modify: `cmd/labtether/main.go:383-394`
- Modify: `cmd/labtether/startup_worker.go:36-51`

**Acceptance Criteria:**
- [ ] All HTTP handlers wrapped with RecoverMiddleware as outermost layer
- [ ] Agent WS dispatcher catches panics per-message without killing the connection
- [ ] All background goroutines use SafeGo with a shared WaitGroup
- [ ] API key touch uses a bounded channel (100) drained by a single goroutine
- [ ] Hub compiles and existing tests pass

**Verify:** `cd /Users/michael/Development/LabTether/hub && go build ./cmd/labtether/ && go test ./cmd/labtether/ -count=1 -timeout 120s 2>&1 | tail -5`

**Steps:**

- [ ] **Step 1: Add RecoverMiddleware as outermost handler wrapper in startup_bootstrap.go**

In `cmd/labtether/startup_bootstrap.go`, after the demo-mode middleware block (around line 330), add a new block that wraps every handler with RecoverMiddleware:

```go
// Wrap every handler with panic recovery as the absolute outermost layer.
// This ensures no unhandled panic in any handler crashes the hub process.
for path, h := range handlers {
    wrapped := servicehttp.RecoverMiddleware(http.HandlerFunc(h))
    handlers[path] = wrapped.ServeHTTP
}
```

Add `"github.com/labtether/labtether/internal/servicehttp"` to the imports.

- [ ] **Step 2: Add panic recovery to WebSocket message dispatcher**

In `cmd/labtether/agent_ws_handler.go`, modify `dispatchAgentWebSocketMessage` (line 407) to recover from panics:

```go
func (s *apiServer) dispatchAgentWebSocketMessage(router shared.WSRouter, assetID string, conn *agentmgr.AgentConn, msg agentmgr.Message) {
	defer func() {
		if err := recover(); err != nil {
			log.Printf("agentws: panic in handler for message type %q from %s: %v\n%s", msg.Type, assetID, err, debug.Stack())
		}
	}()
	handler, ok := router[msg.Type]
	if !ok {
		securityruntime.Logf("agentws: unknown message type %q from %s", msg.Type, assetID)
		return
	}
	handler(conn, msg)
}
```

Add `"runtime/debug"` to imports.

- [ ] **Step 3: Replace bare goroutines in startup_background.go with SafeGo**

Replace the contents of `startRuntimeLoops` to use SafeGo and a WaitGroup. The WaitGroup should be stored on the apiServer struct so shutdown can wait on it.

Add a `backgroundWG sync.WaitGroup` field to apiServer (in the appropriate types file).

Replace the goroutine launches:

```go
func startRuntimeLoops(
	ctx context.Context,
	srv *apiServer,
	pgStore *persistence.PostgresStore,
	workerState *workerRuntimeState,
	retentionTracker *retentionState,
) {
	if srv.terminalPersistentStore != nil {
		if err := srv.terminalPersistentStore.MarkAllAttachedAsDetached(); err != nil {
			log.Printf("labtether: warning: startup recovery for attached persistent sessions failed: %v", err)
		}
	}

	wg := &srv.backgroundWG

	servicehttp.SafeGo(ctx, wg, "heartbeat", func(ctx context.Context) { heartbeatLoop(ctx) })
	servicehttp.SafeGo(ctx, wg, "retention", func(ctx context.Context) { runRetentionLoop(ctx, pgStore, workerState, retentionTracker) })
	startMetricsExport(ctx, srv)
	servicehttp.SafeGo(ctx, wg, "session-cleanup", func(ctx context.Context) { runSessionCleanupLoop(ctx, pgStore) })
	servicehttp.SafeGo(ctx, wg, "alert-evaluator", func(ctx context.Context) { srv.runAlertEvaluator(ctx) })
	servicehttp.SafeGo(ctx, wg, "synthetic-runner", func(ctx context.Context) { srv.runSyntheticRunner(ctx) })
	servicehttp.SafeGo(ctx, wg, "incident-correlator", func(ctx context.Context) { srv.runIncidentCorrelator(ctx) })
	servicehttp.SafeGo(ctx, wg, "hub-collector", func(ctx context.Context) { srv.runHubCollectorLoop(ctx) })
	servicehttp.SafeGo(ctx, wg, "presence-cleanup", func(ctx context.Context) { srv.runPresenceCleanup(ctx) })
	servicehttp.SafeGo(ctx, wg, "web-service-cleanup", func(ctx context.Context) { srv.runWebServiceCleanup(ctx) })
	servicehttp.SafeGo(ctx, wg, "notification-retry", func(ctx context.Context) { srv.runNotificationRetryLoop(ctx) })
	servicehttp.SafeGo(ctx, wg, "protocol-health", func(ctx context.Context) { srv.runProtocolHealthChecker(ctx) })
	servicehttp.SafeGo(ctx, wg, "service-health-linker", func(ctx context.Context) { srv.runServiceHealthLinker(ctx) })

	terminalDeps := srv.ensureTerminalDeps()
	servicehttp.SafeGo(ctx, wg, "archive-worker", func(ctx context.Context) { terminalDeps.StartArchiveWorker(ctx) })

	go func() {
		<-ctx.Done()
		srv.sendShutdownToAgents()
	}()
}
```

- [ ] **Step 4: Bound API key touch goroutines in main.go**

Replace the unbounded `go func()` in `main.go` (around line 387) with a bounded channel pattern. Add to apiServer init:

```go
// In apiServer init or field declaration:
apiKeyTouchCh: make(chan string, 100),
```

Add a SafeGo drainer in startup:

```go
servicehttp.SafeGo(ctx, &srv.backgroundWG, "apikey-touch", func(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            return
        case keyID := <-srv.apiKeyTouchCh:
            touchCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
            if err := srv.apiKeyStore.TouchAPIKeyLastUsed(touchCtx, keyID); err != nil {
                log.Printf("apikey: touch last-used for %s: %v", keyID, err)
            }
            cancel()
        }
    }
})
```

Replace the `go func()` in the auth middleware with a non-blocking channel send:

```go
select {
case srv.apiKeyTouchCh <- key.ID:
default:
    // Channel full — skip this touch to avoid blocking.
}
```

- [ ] **Step 5: Build and run tests**

Run: `cd /Users/michael/Development/LabTether/hub && go build ./cmd/labtether/ && go test ./cmd/labtether/ -count=1 -timeout 120s 2>&1 | tail -5`
Expected: Build succeeds, tests pass

- [ ] **Step 6: Commit**

```bash
cd /Users/michael/Development/LabTether/hub && git add -A && git commit -m "feat: wire panic recovery into hub HTTP, WebSocket, and background loops

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: Hub Database Resilience

**Goal:** Enable pgx connection health checks and add a retryDB helper for internal background operations.

**Files:**
- Modify: `internal/persistence/postgres_schema.go:43-47`
- Create: `internal/persistence/retry.go`
- Create: `internal/persistence/retry_test.go`

**Acceptance Criteria:**
- [ ] pgx HealthCheckPeriod set to 30 seconds
- [ ] retryDB helper retries up to 3 times with exponential backoff (100ms, 500ms, 2s)
- [ ] retryDB respects context cancellation
- [ ] Tests verify retry behavior and backoff timing

**Verify:** `cd /Users/michael/Development/LabTether/hub && go test ./internal/persistence/ -run TestRetryDB -v`

**Steps:**

- [ ] **Step 1: Write tests for retryDB**

```go
// internal/persistence/retry_test.go
package persistence

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
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
```

- [ ] **Step 2: Implement retryDB**

```go
// internal/persistence/retry.go
package persistence

import (
	"context"
	"log"
	"time"
)

// retryBackoffs defines the sleep durations between retry attempts.
var retryBackoffs = []time.Duration{100 * time.Millisecond, 500 * time.Millisecond, 2 * time.Second}

// RetryDB retries fn up to maxAttempts times with exponential backoff.
// Intended for internal background operations (heartbeat, job queue, retention)
// — NOT for user-facing API calls which should fail fast.
func RetryDB(ctx context.Context, maxAttempts int, fn func(ctx context.Context) error) error {
	var lastErr error
	for i := 0; i < maxAttempts; i++ {
		if ctx.Err() != nil {
			if lastErr != nil {
				return lastErr
			}
			return ctx.Err()
		}
		lastErr = fn(ctx)
		if lastErr == nil {
			return nil
		}
		if i < maxAttempts-1 {
			backoff := retryBackoffs[0]
			if i < len(retryBackoffs) {
				backoff = retryBackoffs[i]
			}
			log.Printf("db retry: attempt %d/%d failed: %v (backoff %v)", i+1, maxAttempts, lastErr, backoff)
			select {
			case <-ctx.Done():
				return lastErr
			case <-time.After(backoff):
			}
		}
	}
	return lastErr
}
```

- [ ] **Step 3: Enable HealthCheckPeriod in postgres_schema.go**

In `internal/persistence/postgres_schema.go`, after line 47 (`config.MaxConnIdleTime = ...`), add:

```go
config.HealthCheckPeriod = 30 * time.Second
```

- [ ] **Step 4: Run tests**

Run: `cd /Users/michael/Development/LabTether/hub && go test ./internal/persistence/ -run TestRetryDB -v`
Expected: All 4 tests PASS

- [ ] **Step 5: Commit**

```bash
cd /Users/michael/Development/LabTether/hub && git add internal/persistence/retry.go internal/persistence/retry_test.go internal/persistence/postgres_schema.go && git commit -m "feat: add DB connection health checks and retryDB helper

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: Hub Deeper Health Check

**Goal:** Expand /healthz to return JSON with sub-component status including goroutine count and postgres status.

**Files:**
- Modify: `internal/servicehttp/servicehttp.go:58-72`

**Acceptance Criteria:**
- [ ] /healthz returns JSON with status, postgres, goroutines fields
- [ ] Returns 200 when postgres is reachable, 503 when not
- [ ] goroutines field reflects runtime.NumGoroutine()

**Verify:** `cd /Users/michael/Development/LabTether/hub && go build ./cmd/labtether/`

**Steps:**

- [ ] **Step 1: Update /healthz handler in servicehttp.go**

Replace the /healthz handler (lines 58-72) with:

```go
mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
    status := "ok"
    pgStatus := "ok"
    httpStatus := http.StatusOK
    if cfg.DBPool != nil {
        if err := cfg.DBPool.Ping(r.Context()); err != nil {
            status = "degraded"
            pgStatus = "unreachable"
            httpStatus = http.StatusServiceUnavailable
        }
    } else {
        pgStatus = "not_configured"
    }
    WriteJSON(w, httpStatus, map[string]any{
        "service":    cfg.Name,
        "status":     status,
        "postgres":   pgStatus,
        "goroutines": runtime.NumGoroutine(),
        "timestamp":  time.Now().UTC().Format(time.RFC3339),
    })
})
```

Add `"runtime"` to imports.

- [ ] **Step 2: Build and verify**

Run: `cd /Users/michael/Development/LabTether/hub && go build ./cmd/labtether/`
Expected: Build succeeds

- [ ] **Step 3: Commit**

```bash
cd /Users/michael/Development/LabTether/hub && git add internal/servicehttp/servicehttp.go && git commit -m "feat: deepen /healthz to report postgres status and goroutine count

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: Agent Panic Recovery (safeGo + all goroutine entry points)

**Goal:** Add panic recovery to all agent goroutine entry points: message handlers, transport loops, and runtime loops.

**Files:**
- Create: `internal/agentcore/safego.go`
- Create: `internal/agentcore/safego_test.go`
- Modify: `internal/agentcore/command_handler.go:82-154`
- Modify: `internal/agentcore/ws_transport.go:269-294`
- Modify: `internal/agentcore/runtime.go:151-201`

**Acceptance Criteria:**
- [ ] safeGo helper with recover + restart (mirrors hub pattern)
- [ ] All `go func()` handlers in receiveLoop wrapped with recover
- [ ] pingLoop wrapped with recover
- [ ] collectLoop and heartbeatLoop wrapped with recover
- [ ] WaitGroup tracks handler goroutines; receiveLoop waits on disconnect
- [ ] All handlers go through semaphore (not just "heavy" ones)

**Verify:** `cd /Users/michael/Development/LabTether/labtether-agent && go test ./internal/agentcore/ -run TestSafeGo -v`

**Steps:**

- [ ] **Step 1: Create safego.go and safego_test.go in agent repo**

```go
// internal/agentcore/safego.go
package agentcore

import (
	"context"
	"log"
	"runtime/debug"
	"sync"
	"time"
)

// SafeGo launches fn in a goroutine with panic recovery. If fn panics, the
// panic is logged and fn is restarted after a 1-second backoff. Exits when
// ctx is cancelled.
func SafeGo(ctx context.Context, wg *sync.WaitGroup, name string, fn func(ctx context.Context)) {
	if wg != nil {
		wg.Add(1)
	}
	go func() {
		if wg != nil {
			defer wg.Done()
		}
		for {
			if ctx.Err() != nil {
				return
			}
			func() {
				defer func() {
					if err := recover(); err != nil {
						log.Printf("safego[%s]: panic recovered: %v\n%s", name, err, debug.Stack())
					}
				}()
				fn(ctx)
			}()
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
		}
	}()
}

// safeHandler wraps fn with panic recovery for use in message handler goroutines.
// On panic, it logs the error and stack trace. The deferred sem drain and wg.Done
// should be set up by the caller.
func safeHandler(name string, fn func()) {
	defer func() {
		if err := recover(); err != nil {
			log.Printf("handler[%s]: panic recovered: %v\n%s", name, err, debug.Stack())
		}
	}()
	fn()
}
```

Test file mirrors the hub pattern — see Task 1 for the test structure. Copy and adapt for `package agentcore`.

- [ ] **Step 2: Add WaitGroup and semaphore to ALL handlers in command_handler.go**

In `receiveLoop`, add a `sync.WaitGroup` before the for loop. For every `go func()` handler:
1. Add `wg.Add(1)` before the goroutine
2. Add `defer wg.Done()` inside the goroutine
3. Wrap the handler call with `safeHandler`
4. Move ALL handlers (including lightweight ones like MsgPing, MsgConfigUpdate) through the semaphore

After the for loop exits (ctx.Done), add a wait with timeout:

```go
// Wait for in-flight handlers to drain.
done := make(chan struct{})
go func() { wg.Wait(); close(done) }()
select {
case <-done:
case <-time.After(5 * time.Second):
    log.Printf("agentws: timed out waiting for %d handlers to drain", handlerCount())
}
```

- [ ] **Step 3: Add recover to pingLoop in ws_transport.go**

Wrap the body of `pingLoop` (lines 270-293):

```go
func (t *wsTransport) pingLoop(conn *websocket.Conn, done chan struct{}) {
	defer func() {
		if err := recover(); err != nil {
			log.Printf("agentws: panic in pingLoop: %v\n%s", err, debug.Stack())
		}
	}()
	// ... existing body unchanged ...
}
```

- [ ] **Step 4: Add recover to collectLoop and heartbeatLoop in runtime.go**

Wrap each loop body the same way. The loops already have for/select patterns — add defer recover at the top of each function.

- [ ] **Step 5: Build and test**

Run: `cd /Users/michael/Development/LabTether/labtether-agent && go build ./... && go test ./internal/agentcore/ -run TestSafeGo -v`
Expected: Build succeeds, tests pass

- [ ] **Step 6: Commit**

```bash
cd /Users/michael/Development/LabTether/labtether-agent && git add -A && git commit -m "feat: add panic recovery to all agent goroutine entry points

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

### Task 6: Agent Command Execution Kill-on-Timeout

**Goal:** Force-kill processes that survive context timeout and add timeout to Docker exec sessions.

**Files:**
- Modify: `internal/agentcore/remoteaccess/exec_ws.go:66-96`
- Modify: `internal/agentcore/docker/exec.go:82`

**Acceptance Criteria:**
- [ ] After context deadline, cmd.Process.Kill() is called explicitly
- [ ] Docker exec sessions use context.WithTimeout (5 minutes) instead of context.WithCancel
- [ ] No zombie processes after command timeout

**Verify:** `cd /Users/michael/Development/LabTether/labtether-agent && go build ./...`

**Steps:**

- [ ] **Step 1: Add force-kill on timeout in exec_ws.go**

Replace the command execution block in `HandleCommandRequest` (around lines 69-95):

```go
ctx, cancel := context.WithTimeout(context.Background(), timeout)
defer cancel()

cmd, err := securityruntime.NewCommandContext(ctx, "sh", "-lc", req.Command)
if err != nil {
    log.Printf("agentws: command blocked by runtime policy: %v", err)
    sendCommandResult(transport, req, "failed", err.Error())
    return
}

output, err := cmd.CombinedOutput()

// Force-kill if the process survived the context timeout.
if ctx.Err() == context.DeadlineExceeded && cmd.Process != nil {
    _ = cmd.Process.Kill()
    _, _ = cmd.Process.Wait() // Reap zombie.
}

status := "succeeded"
outputStr := TruncateCommandOutput(output, MaxCommandOutputBytes)
if ctx.Err() == context.DeadlineExceeded {
    status = "failed"
    if outputStr != "" {
        outputStr += "\nerror: command timed out"
    } else {
        outputStr = "command timed out"
    }
} else if err != nil {
    status = "failed"
    if outputStr != "" {
        outputStr += "\nerror: " + err.Error()
    } else {
        outputStr = err.Error()
    }
}

sendCommandResult(transport, req, status, outputStr)
```

Apply the same kill pattern to `HandleUpdateRequest` for its command execution paths.

- [ ] **Step 2: Add timeout to Docker exec sessions in exec.go**

In `HandleExecStart` (line 82), replace:

```go
ctx, cancel := context.WithCancel(context.Background())
```

with:

```go
const dockerExecTimeout = 5 * time.Minute
ctx, cancel := context.WithTimeout(context.Background(), dockerExecTimeout)
```

The existing `cancel` is stored on the session and called on cleanup, so the rest of the flow works unchanged. When the timeout fires, the context cancellation propagates to the hijacked connection operations.

- [ ] **Step 3: Build**

Run: `cd /Users/michael/Development/LabTether/labtether-agent && go build ./...`
Expected: Build succeeds

- [ ] **Step 4: Commit**

```bash
cd /Users/michael/Development/LabTether/labtether-agent && git add -A && git commit -m "feat: force-kill timed-out commands and add Docker exec timeout

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

---

### Task 7: Agent Watchdog

**Goal:** Add a watchdog goroutine that detects stuck main loops and goroutine leaks.

**Files:**
- Create: `internal/agentcore/watchdog.go`
- Create: `internal/agentcore/watchdog_test.go`
- Modify: `internal/agentcore/runtime.go:177-199` (bump heartbeat counter)
- Modify: `internal/agentcore/run.go` (wire watchdog)

**Acceptance Criteria:**
- [ ] Watchdog checks a heartbeat counter every 60 seconds
- [ ] If counter hasn't changed in 5 minutes, logs error and exits with code 11
- [ ] Goroutine count exceeding 500 with >50% growth logs a warning
- [ ] Tests verify stuck detection and goroutine leak warning

**Verify:** `cd /Users/michael/Development/LabTether/labtether-agent && go test ./internal/agentcore/ -run TestWatchdog -v`

**Steps:**

- [ ] **Step 1: Write tests**

```go
// internal/agentcore/watchdog_test.go
package agentcore

import (
	"bytes"
	"context"
	"log"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestWatchdog_DetectsStuck(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(nil)

	var counter atomic.Int64
	var exitCode atomic.Int32

	cfg := WatchdogConfig{
		HeartbeatCounter: &counter,
		CheckInterval:    100 * time.Millisecond,
		StuckThreshold:   300 * time.Millisecond,
		ExitFunc:         func(code int) { exitCode.Store(int32(code)) },
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go RunWatchdog(ctx, cfg)

	// Don't bump the counter — watchdog should detect stuck.
	time.Sleep(time.Second)

	if exitCode.Load() != 11 {
		t.Fatalf("expected exit code 11, got %d", exitCode.Load())
	}
	if !strings.Contains(buf.String(), "stuck") {
		t.Errorf("expected 'stuck' in log, got: %s", buf.String())
	}
}

func TestWatchdog_HealthyDoesNotExit(t *testing.T) {
	var counter atomic.Int64
	var exitCode atomic.Int32

	cfg := WatchdogConfig{
		HeartbeatCounter: &counter,
		CheckInterval:    50 * time.Millisecond,
		StuckThreshold:   500 * time.Millisecond,
		ExitFunc:         func(code int) { exitCode.Store(int32(code)) },
	}

	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()

	// Bump counter periodically.
	go func() {
		for ctx.Err() == nil {
			counter.Add(1)
			time.Sleep(30 * time.Millisecond)
		}
	}()

	RunWatchdog(ctx, cfg)

	if exitCode.Load() != 0 {
		t.Fatalf("expected no exit, got code %d", exitCode.Load())
	}
}
```

- [ ] **Step 2: Implement watchdog**

```go
// internal/agentcore/watchdog.go
package agentcore

import (
	"context"
	"log"
	"runtime"
	"sync/atomic"
	"time"
)

const (
	watchdogExitCode          = 11
	defaultWatchdogInterval   = 60 * time.Second
	defaultStuckThreshold     = 5 * time.Minute
	goroutineWarningThreshold = 500
)

// WatchdogConfig configures the watchdog goroutine.
type WatchdogConfig struct {
	HeartbeatCounter *atomic.Int64
	CheckInterval    time.Duration
	StuckThreshold   time.Duration
	ExitFunc         func(code int) // os.Exit in production, mock in tests
}

// RunWatchdog monitors the heartbeat counter and goroutine count.
// If the heartbeat counter hasn't changed for StuckThreshold, it logs
// an error and calls ExitFunc(11). Exits when ctx is cancelled.
func RunWatchdog(ctx context.Context, cfg WatchdogConfig) {
	if cfg.CheckInterval == 0 {
		cfg.CheckInterval = defaultWatchdogInterval
	}
	if cfg.StuckThreshold == 0 {
		cfg.StuckThreshold = defaultStuckThreshold
	}

	ticker := time.NewTicker(cfg.CheckInterval)
	defer ticker.Stop()

	lastValue := cfg.HeartbeatCounter.Load()
	lastChange := time.Now()
	lastGoroutines := runtime.NumGoroutine()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			current := cfg.HeartbeatCounter.Load()
			if current != lastValue {
				lastValue = current
				lastChange = time.Now()
			} else if time.Since(lastChange) > cfg.StuckThreshold {
				log.Printf("watchdog: heartbeat stuck for %v (counter=%d), exiting with code %d",
					time.Since(lastChange).Round(time.Second), current, watchdogExitCode)
				buf := make([]byte, 64*1024)
				n := runtime.Stack(buf, true)
				log.Printf("watchdog: goroutine dump:\n%s", buf[:n])
				if cfg.ExitFunc != nil {
					cfg.ExitFunc(watchdogExitCode)
				}
				return
			}

			// Goroutine leak detection.
			numGoroutines := runtime.NumGoroutine()
			if numGoroutines > goroutineWarningThreshold && lastGoroutines > 0 {
				growth := float64(numGoroutines-lastGoroutines) / float64(lastGoroutines)
				if growth > 0.5 {
					log.Printf("watchdog: goroutine count %d (was %d, +%.0f%%) — possible leak",
						numGoroutines, lastGoroutines, growth*100)
				}
			}
			lastGoroutines = numGoroutines
		}
	}
}
```

- [ ] **Step 3: Bump heartbeat counter in runtime.go heartbeatLoop**

In `runtime.go`, add an `atomic.Int64` field to the `Runtime` struct called `HeartbeatCounter`. In `heartbeatLoop`, after `r.publishOnce(ctx)` (line 189), add:

```go
r.HeartbeatCounter.Add(1)
```

- [ ] **Step 4: Wire watchdog in run.go**

In the `Run` function (around where `runtime` is created), start the watchdog:

```go
go RunWatchdog(ctx, WatchdogConfig{
    HeartbeatCounter: &runtime.HeartbeatCounter,
    ExitFunc:         os.Exit,
})
```

- [ ] **Step 5: Test**

Run: `cd /Users/michael/Development/LabTether/labtether-agent && go test ./internal/agentcore/ -run TestWatchdog -v`
Expected: Both tests PASS

- [ ] **Step 6: Commit**

```bash
cd /Users/michael/Development/LabTether/labtether-agent && git add -A && git commit -m "feat: add watchdog for stuck heartbeat and goroutine leak detection

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"
```

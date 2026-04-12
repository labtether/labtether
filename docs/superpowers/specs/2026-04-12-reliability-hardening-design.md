# Reliability Hardening — Hub & Linux Agent

**Date:** 2026-04-12
**Scope:** Critical path reliability for single-hub, 10-50 agent deployments.
**Goal:** Prevent crashes, hangs, goroutine leaks, and silent failures.

## 1. Panic Recovery

### Hub

**HTTP handlers:** Add `RecoverMiddleware` in `internal/servicehttp/` wrapping all HTTP handlers. On panic: log stack trace, return 500 with generic error body, increment a counter. Applied once at the top of the handler chain in `startup_bootstrap.go`.

**Agent WebSocket dispatcher:** Add recover() inside the message dispatch loop in `agent_ws_handler.go`. On panic: log the message type + stack trace, send an error response to the agent, continue the read loop (don't kill the connection).

**Background goroutines:** Add a `safeGo(ctx, name, fn)` helper in `internal/servicehttp/` that wraps `fn` with recover(), logs panics with the goroutine name, and restarts the function after a 1-second backoff. Used in `startup_background.go` for all 12+ background loops.

### Agent

**Message handlers:** Add recover() in each `go func()` handler spawned by `receiveLoop` in `command_handler.go`. On panic: log stack trace, release semaphore, send error response to hub.

**Transport goroutines:** Add recover() to `pingLoop`, `reconnectLoop` in `ws_transport.go`. On panic in pingLoop: log and let the read deadline expire (connection will close naturally). On panic in reconnectLoop: log and exit (service manager restarts).

**Runtime loops:** Add recover() to `collectLoop` and `heartbeatLoop` in `runtime.go`. On panic: log, backoff 1 second, restart the loop.

## 2. Command Execution Lifecycle (Agent)

**Force-kill on timeout:** In `exec_ws.go`, after `cmd.CombinedOutput()` returns `context.DeadlineExceeded`, explicitly call `cmd.Process.Kill()` followed by `cmd.Wait()` to reap the zombie. Same pattern for update commands.

**Docker exec timeout:** In `docker/exec.go`, wrap the exec session read loop with a context timeout (5-minute default, matching regular commands). Cancel the exec on timeout.

**Output cap:** `CombinedOutput()` already has a 64KB cap via `MaxCommandOutputBytes`. Verify this is enforced in all code paths. If any path buffers unbounded output, add a `LimitReader`.

## 3. Goroutine Supervision

### Hub

**Background loop supervision:** Replace bare `go func()` calls in `startup_background.go` with `safeGo()` from section 1. This gives automatic panic recovery + restart.

**Shutdown coordination:** Add a `sync.WaitGroup` to track background goroutines. `safeGo` increments on launch, decrements on exit. Shutdown waits on the WaitGroup (with a timeout) before exiting.

**Bounded fire-and-forget:** The API key "last used" touch in `main.go` spawns unbounded goroutines. Replace with a buffered channel (size 100) drained by a single background goroutine.

### Agent

**Handler tracking:** Add a `sync.WaitGroup` to `receiveLoop` in `command_handler.go`. Each spawned handler increments on start, decrements on finish. On disconnect, wait (5-second timeout) for handlers to drain before returning.

**Lightweight handler limit:** Move the semaphore check to cover ALL handler goroutines in `receiveLoop`, not just the "heavy" ones. Use the existing `maxConcurrentHandlers = 20` limit.

## 4. Database Resilience (Hub)

**Connection health checks:** Enable pgx's `HealthCheckPeriod` (30 seconds) in `internal/persistence/postgres_schema.go`. This evicts dead connections from the pool automatically after a postgres restart.

**Retry on critical internal operations:** Add a `retryDB(ctx, maxAttempts, fn)` helper in `internal/persistence/`. Wraps `fn` with 3 attempts and exponential backoff (100ms, 500ms, 2s). Applied ONLY to internal background operations: heartbeat writes, job queue polls, retention prune. NOT applied to user-facing API calls (those fail fast with 503).

**Deeper health check:** Expand `/healthz` to return JSON with sub-component status:
```json
{
  "status": "ok",
  "postgres": "ok",
  "goroutines": 47,
  "agents_connected": 12,
  "background_loops": "ok"
}
```
Return 200 if postgres is reachable. Return 503 if postgres ping fails. The extra fields are informational (don't affect the HTTP status).

## 5. Agent Watchdog

**Progress tracking:** Add a monotonically incrementing `atomic.Int64` counter bumped by `heartbeatLoop` on each tick. A watchdog goroutine checks the counter every 60 seconds. If it hasn't changed in 5 minutes, log an error with goroutine dump and exit with code 11 (distinct from update exit code 10). Systemd restarts the agent.

**Goroutine leak detection:** In the watchdog goroutine, also check `runtime.NumGoroutine()`. If it exceeds 500 and has grown by >50% since last check, log a warning with `runtime.Stack()` output. No exit — just visibility.

## Out of Scope

- Distributed rate limiting (single hub deployment)
- Self-update rollback (systemd `StartLimitBurst` handles crash loops)
- Message buffering during disconnect (only telemetry needs this, and it already has a ring buffer)
- Security hardening (separate initiative)
- Structured logging migration (too large, separate project)

## Files Changed

### Hub (`/Users/michael/Development/LabTether/hub`)
- `internal/servicehttp/recover.go` — NEW: RecoverMiddleware + safeGo helper
- `internal/servicehttp/servicehttp.go` — Deeper /healthz, apply RecoverMiddleware
- `cmd/labtether/startup_bootstrap.go` — Wire RecoverMiddleware into handler chain
- `cmd/labtether/startup_background.go` — Replace bare goroutines with safeGo, add WaitGroup
- `cmd/labtether/agent_ws_handler.go` — Add recover() to message dispatcher
- `cmd/labtether/main.go` — Bounded API key touch channel
- `internal/persistence/postgres_schema.go` — Enable HealthCheckPeriod
- `internal/persistence/retry.go` — NEW: retryDB helper
- `cmd/labtether/startup_worker.go` — Use retryDB for job queue polls

### Agent (`/Users/michael/Development/LabTether/labtether-agent`)
- `internal/agentcore/safego.go` — NEW: safeGo + recover helpers
- `internal/agentcore/command_handler.go` — Panic recovery in handlers, WaitGroup for drain
- `internal/agentcore/ws_transport.go` — Panic recovery in pingLoop/reconnectLoop
- `internal/agentcore/runtime.go` — Panic recovery in collectLoop/heartbeatLoop
- `internal/agentcore/watchdog.go` — NEW: watchdog goroutine
- `internal/agentcore/run.go` — Wire watchdog, pass WaitGroup
- `internal/agentcore/remoteaccess/exec_ws.go` — Force-kill on timeout
- `internal/agentcore/docker/exec.go` — Add context timeout to exec sessions

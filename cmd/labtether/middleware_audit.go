package main

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/securityruntime"
)

// statusCapturingWriter wraps an http.ResponseWriter to capture the HTTP
// status code written by the handler. If WriteHeader is never called, the
// status defaults to 200 (the net/http implicit default).
type statusCapturingWriter struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

func (w *statusCapturingWriter) WriteHeader(code int) {
	if !w.written {
		w.statusCode = code
		w.written = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusCapturingWriter) Write(b []byte) (int, error) {
	if !w.written {
		w.statusCode = http.StatusOK
		w.written = true
	}
	return w.ResponseWriter.Write(b)
}

// Unwrap returns the underlying ResponseWriter so that http.ResponseController
// can walk the wrapper chain to discover http.Hijacker and other optional
// interfaces (same pattern as GzipResponseWriter.Unwrap).
func (w *statusCapturingWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// Flush delegates to the underlying writer if it implements http.Flusher,
// matching the GzipResponseWriter pattern for SSE compatibility.
func (w *statusCapturingWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack preserves WebSocket and other upgraded connections through the
// audit wrapper. Libraries such as gorilla/websocket use a direct
// http.Hijacker type assertion and do not walk Unwrap chains.
func (w *statusCapturingWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	conn, rw, err := hijacker.Hijack()
	if err == nil && !w.written {
		w.statusCode = http.StatusSwitchingProtocols
		w.written = true
	}
	return conn, rw, err
}

// isAuditSkippedPath reports whether the given path should be excluded from
// audit logging. Health probes, readiness checks, and version endpoints
// generate high-frequency noise with no security or operational value.
func isAuditSkippedPath(path string) bool {
	switch path {
	case "/healthz", "/readyz", "/version":
		return true
	}
	// Static assets served by the Next.js console frontend.
	if strings.HasPrefix(path, "/_next/") {
		return true
	}
	if strings.HasPrefix(path, "/static/") {
		return true
	}
	// Favicon and manifest files.
	if path == "/favicon.ico" || path == "/manifest.json" {
		return true
	}
	return false
}

// auditNote travels in the request context so the audit middleware learns
// what the auth layer decided: who signed in (contextWithPrincipal) and
// whether the route only reads although it uses POST (withReadCapabilityAuth).
type auditNote struct {
	mu       sync.Mutex
	actorID  string
	readOnly bool
}

type auditNoteKey struct{}

func noteAuditActor(ctx context.Context, actorID string) {
	if note, ok := ctx.Value(auditNoteKey{}).(*auditNote); ok {
		note.mu.Lock()
		note.actorID = strings.TrimSpace(actorID)
		note.mu.Unlock()
	}
}

func noteAuditReadOnly(ctx context.Context) {
	if note, ok := ctx.Value(auditNoteKey{}).(*auditNote); ok {
		note.mu.Lock()
		note.readOnly = true
		note.mu.Unlock()
	}
}

func (n *auditNote) read() (actorID string, readOnly bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.actorID, n.readOnly
}

// keepRequestAudit reports whether a request belongs in the audit log.
// Reads go to the hub log only: the console polls several of them every few
// seconds, and storing each one buried people's actions under thousands of
// "API request GET" rows. Changes are kept whatever their result, so failed
// sign-ins and refused changes stay visible. Read-only capabilities (event
// stream tickets, MCP calls, which audit their tools themselves) and agent
// heartbeats are machine traffic, not something a person did.
func keepRequestAudit(method, path string, readOnly bool) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		return false
	}
	return !readOnly && path != "/assets/heartbeat"
}

// requestAuditDecision is the result word the audit log shows for a request.
func requestAuditDecision(status int) string {
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden || status == http.StatusTooManyRequests:
		return "denied"
	case status >= http.StatusInternalServerError:
		return "failed"
	}
	return ""
}

// auditMiddleware logs every API request (method, path, status, duration and
// client address) to the hub log, and appends the changes among them to the
// audit store (see keepRequestAudit), attributed to the person or key that
// made them. Requests nobody signed in for keep the client address as actor.
//
// It wraps handlers outside the per-handler auth wrappers (withAuth), so it
// learns the signed-in actor through the auditNote in the request context.
func (s *apiServer) auditMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Skip non-API paths to avoid noisy audit entries.
		if isAuditSkippedPath(path) {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		sw := &statusCapturingWriter{ResponseWriter: w, statusCode: http.StatusOK}
		note := &auditNote{}

		next.ServeHTTP(sw, r.WithContext(context.WithValue(r.Context(), auditNoteKey{}, note)))

		duration := time.Since(start)
		clientIP, peerIP := auditRequestAddresses(r)

		securityruntime.Logf("audit: %s %s %d %dms ip=%s peer=%s",
			r.Method, path, sw.statusCode, duration.Milliseconds(), clientIP, peerIP)

		actorID, readOnly := note.read()
		if s.auditStore == nil || !keepRequestAudit(r.Method, path, readOnly) {
			return
		}
		if actorID == "" {
			actorID = clientIP
		}
		// Best-effort append to the audit store -- never fail the request.
		event := audit.Event{
			ID:        idgen.New("audit"),
			Type:      "api.request",
			ActorID:   actorID,
			Target:    r.Method + " " + path,
			Decision:  requestAuditDecision(sw.statusCode),
			Timestamp: start.UTC(),
			Details: map[string]any{
				"method":      r.Method,
				"path":        path,
				"status":      sw.statusCode,
				"duration_ms": duration.Milliseconds(),
				"client_ip":   clientIP,
				"peer_ip":     peerIP,
			},
		}
		select {
		case s.auditEventCh <- event:
		default:
			// The request path must not create unbounded goroutines or block on
			// persistence. A saturated queue intentionally drops low-value
			// request telemetry; handler-level security audit events remain.
		}
	})
}

// auditRequestAddresses returns the client to record and the direct peer.
// Through the console (or another trusted proxy) the client is the address
// that proxy appended to X-Forwarded-For; otherwise it is the peer. Rate
// limits keep their own keys. Requests without the header skip the trusted
// proxy check and its name lookup.
func auditRequestAddresses(r *http.Request) (client, peer string) {
	peer = requestClientKey(r)
	if strings.TrimSpace(r.Header.Get("X-Forwarded-For")) == "" {
		return peer, peer
	}
	return trustedRequestClientIP(r), peer
}

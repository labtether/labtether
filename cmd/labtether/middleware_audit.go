package main

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/idgen"
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

// auditMiddleware returns an HTTP middleware that logs every API request with
// method, path, status code, duration, and authenticated actor. It also
// appends a best-effort audit event to the audit store for API-queryable
// trail.
//
// The middleware must wrap handlers *after* auth middleware has run (so that
// the actor ID is present in the request context) but the typical deployment
// inserts it between CORS and gzip in the middleware chain -- this is fine
// because the auth context is set by per-handler wrappers (withAuth), not a
// global middleware layer.
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

		next.ServeHTTP(sw, r)

		duration := time.Since(start)
		// Note: actor ID is not available at middleware level because auth middleware
		// creates a new request context. Per-handler audit events (appendAuditEventBestEffort)
		// capture the authenticated actor. This middleware log uses client IP for correlation.
		clientIP := requestClientKey(r)

		log.Printf("audit: %s %s %d %dms ip=%s",
			r.Method, path, sw.statusCode, duration.Milliseconds(), clientIP)

		// Best-effort append to the audit store -- never fail the request.
		if s.auditStore != nil {
			event := audit.Event{
				ID:        idgen.New("audit"),
				Type:      "api.request",
				ActorID:   clientIP,
				Target:    r.Method + " " + path,
				Timestamp: start.UTC(),
				Details: map[string]any{
					"method":      r.Method,
					"path":        path,
					"status":      sw.statusCode,
					"duration_ms": duration.Milliseconds(),
					"client_ip":   clientIP,
				},
			}
			go func() {
				if err := s.auditStore.Append(event); err != nil {
					log.Printf("audit: failed to append event: %v", err)
				}
			}()
		}
	})
}

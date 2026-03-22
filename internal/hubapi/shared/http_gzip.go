package shared

import (
	"compress/gzip"
	"net/http"
	"strings"
)

// GzipResponseWriter wraps an http.ResponseWriter to transparently compress
// response bodies using gzip. It delegates to the underlying writer for all
// header manipulation and conditionally passes writes through the gzip encoder.
//
// It implements http.Flusher when the underlying writer does, which is
// required for SSE (Server-Sent Events) streams that flush incrementally.
//
// http.Hijacker detection is handled via the Unwrap method: the net/http
// package's ResponseController walks the Unwrap chain to locate Hijacker, so
// explicitly implementing Hijacker here is not needed.
type GzipResponseWriter struct {
	http.ResponseWriter
	Gz *gzip.Writer
}

// Write compresses p through the gzip writer instead of writing directly to
// the underlying transport.
func (w *GzipResponseWriter) Write(p []byte) (int, error) {
	return w.Gz.Write(p)
}

// Flush flushes the gzip encoder's internal buffer and then flushes the
// underlying http.ResponseWriter if it implements http.Flusher. This is
// essential for SSE endpoints that send chunked updates.
func (w *GzipResponseWriter) Flush() {
	// Best-effort: ignore errors from the gzip flush since they will surface
	// on the next Write or Close anyway.
	_ = w.Gz.Flush()
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap returns the underlying ResponseWriter so that http.ResponseController
// (and the net/http internals) can walk the wrapper chain to discover
// http.Hijacker and other optional interfaces.
func (w *GzipResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// GzipMiddleware is an HTTP middleware that compresses response bodies with
// gzip when the client advertises Accept-Encoding: gzip support.
//
// WebSocket upgrade requests and known streaming paths are always passed
// through uncompressed because:
//   - WebSocket frames are already framed; gzip would corrupt the upgrade.
//   - Desktop and terminal stream paths perform their own binary framing.
func GzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip compression for WebSocket upgrade handshakes. The Connection
		// header contains "upgrade" (case-insensitive) for all WS requests.
		if IsWebSocketUpgrade(r) {
			next.ServeHTTP(w, r)
			return
		}

		// Skip known streaming path prefixes even when no Upgrade header is
		// present (for example, ticket-authenticated stream fetches that
		// arrive before the actual WS upgrade).
		if IsGzipExcludedPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		// Only compress when the client explicitly accepts gzip.
		if !ClientAcceptsGzip(r) {
			next.ServeHTTP(w, r)
			return
		}

		gz, err := gzip.NewWriterLevel(w, gzip.BestSpeed)
		if err != nil {
			// Extremely unlikely (only fails on invalid level constant).
			// Fall back to uncompressed rather than returning a 500.
			next.ServeHTTP(w, r)
			return
		}
		defer gz.Close()

		// Advertise the encoding and invalidate any pre-computed content length
		// because the compressed size will differ from the original.
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Vary", "Accept-Encoding")
		w.Header().Del("Content-Length")

		next.ServeHTTP(&GzipResponseWriter{ResponseWriter: w, Gz: gz}, r)
	})
}

// ClientAcceptsGzip reports whether the request's Accept-Encoding header
// includes "gzip". It intentionally does not parse quality values (q=) -- a
// client that lists gzip anywhere in Accept-Encoding is treated as accepting
// it, which matches real-world browser and HTTP/1.1 client behavior.
func ClientAcceptsGzip(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept-Encoding"), "gzip")
}

// IsWebSocketUpgrade reports whether r is a WebSocket upgrade request by
// checking both the Upgrade and Connection headers per RFC 6455 section 4.1.
func IsWebSocketUpgrade(r *http.Request) bool {
	if !strings.EqualFold(strings.TrimSpace(r.Header.Get("Upgrade")), "websocket") {
		return false
	}
	return strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade")
}

// IsGzipExcludedPath reports whether path should bypass gzip compression.
// These are paths that carry binary-framed or already-compressed data, or that
// perform WebSocket upgrades that may arrive without the Upgrade header set
// before the handshake is fully negotiated.
func IsGzipExcludedPath(path string) bool {
	// Agent WebSocket transport.
	if path == "/ws/agent" {
		return true
	}
	// Browser event stream (SSE + WebSocket hybrid endpoint).
	if path == "/ws/events" {
		return true
	}
	// Desktop and terminal streaming sessions (binary-framed WS + audio sideband).
	if strings.HasPrefix(path, "/desktop/sessions/") {
		return true
	}
	if strings.HasPrefix(path, "/terminal/sessions/") {
		return true
	}
	return false
}

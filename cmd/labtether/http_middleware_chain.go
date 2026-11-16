package main

import (
	"net/http"

	"github.com/labtether/labtether/internal/servicehttp"
)

// wrapHTTPHandlers applies the hub-wide middleware to every route handler.
// The body was moved unchanged from runHub so tests can build the real chain.
// Each loop wraps the result of the one before it, so the last loop is the
// outermost layer: panic recovery, then demo read-only, CORS, audit and gzip.
func wrapHTTPHandlers(srv *apiServer, handlers map[string]http.HandlerFunc) map[string]http.HandlerFunc {
	// Wrap every handler with gzip compression. The middleware is path-aware
	// and skips WebSocket upgrade paths and known binary-framed stream routes
	// so that compression is only applied to regular JSON API responses.
	for path, h := range handlers {
		handlers[path] = gzipMiddleware(h).ServeHTTP
	}

	// Wrap every handler with audit logging middleware. This runs after CORS
	// (applied next) but before gzip so it captures the real response status
	// code. It logs method, path, status, duration, and authenticated actor
	// for every API request and appends best-effort audit events to the store.
	for path, h := range handlers {
		wrapped := srv.auditMiddleware(http.HandlerFunc(h))
		handlers[path] = wrapped.ServeHTTP
	}

	// Wrap every handler with CORS middleware. This runs as the outermost
	// layer so that OPTIONS preflight requests receive proper CORS headers
	// and a 204 response without passing through auth middleware.
	for path, h := range handlers {
		wrapped := srv.corsMiddleware(http.HandlerFunc(h))
		handlers[path] = wrapped.ServeHTTP
	}

	// In demo mode, wrap every handler with read-only middleware as the
	// outermost layer. This blocks all mutating requests (POST/PUT/DELETE/PATCH)
	// except for explicitly allowlisted paths like /api/demo/session.
	if srv.demoMode {
		for path, h := range handlers {
			finalHandler := demoReadOnlyMiddleware(http.HandlerFunc(h))
			handlers[path] = finalHandler.ServeHTTP
		}
	}

	// Wrap every handler with panic recovery as the absolute outermost layer.
	for path, h := range handlers {
		wrapped := servicehttp.RecoverMiddleware(http.HandlerFunc(h))
		handlers[path] = wrapped.ServeHTTP
	}

	return handlers
}

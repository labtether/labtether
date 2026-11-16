package main

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/audit"
)

// TestWiringPinMiddlewareOrder pins the order wrapHTTPHandlers applies the
// hub-wide middleware in, using probe handlers inside the real chain. From
// the outside in it is: panic recovery, demo read-only (demo mode only),
// CORS and browser-origin check, request audit, gzip, then the route.
//
// Risk covered: a C2 move that reorders the layers or drops one. What each
// check shows:
//   - recovery is outermost: a panic answers 500, and audit (inside it) logs
//     nothing for that request;
//   - demo is outside CORS: a cross-origin demo write gets the demo refusal,
//     not "forbidden origin";
//   - CORS answers a same-origin preflight with 204 before the route runs;
//   - audit is inside CORS: a write CORS refuses leaves no audit record,
//     while audit records the route's own status;
//   - gzip compresses an answer when the client accepts it.
//
// It cannot tell audit-inside-gzip from gzip-inside-audit: both record the
// same status. No golden file; the expectations are written here.
func TestWiringPinMiddlewareOrder(t *testing.T) {
	t.Setenv("LABTETHER_CORS_ALLOWED_ORIGINS", "")
	const crossOrigin = "https://other.example"
	const sameOrigin = "http://example.com" // httptest.NewRequest uses Host example.com

	t.Run("panic is answered by recovery outside audit", func(t *testing.T) {
		p := newMiddlewarePin(t, false)
		rec := p.serve(http.MethodPost, "/pin/panic", nil)
		if rec.Code != http.StatusInternalServerError || strings.TrimSpace(rec.Body.String()) != `{"error":"internal server error"}` {
			t.Fatalf("panic answer = %d %q, want recovery's 500", rec.Code, rec.Body.String())
		}
		if events := p.auditEvents(); len(events) != 0 {
			t.Fatalf("audit logged a request that panicked: %+v (recovery must be outside audit)", events)
		}
	})

	t.Run("demo refuses a write before CORS runs", func(t *testing.T) {
		p := newMiddlewarePin(t, true)
		rec := p.serve(http.MethodPost, "/pin/ok", http.Header{"Origin": {crossOrigin}})
		if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), `"demo":true`) {
			t.Fatalf("demo cross-origin write = %d %q, want the demo refusal (demo outside CORS)", rec.Code, rec.Body.String())
		}
		if p.reached {
			t.Fatal("route ran for a demo write")
		}
		if events := p.auditEvents(); len(events) != 0 {
			t.Fatalf("audit logged a demo refusal: %+v", events)
		}
	})

	t.Run("CORS answers a preflight before the route", func(t *testing.T) {
		p := newMiddlewarePin(t, true)
		rec := p.serve(http.MethodOptions, "/pin/ok", http.Header{"Origin": {sameOrigin}})
		if rec.Code != http.StatusNoContent || rec.Header().Get("Access-Control-Allow-Origin") != sameOrigin {
			t.Fatalf("preflight = %d allow-origin=%q, want 204 from CORS", rec.Code, rec.Header().Get("Access-Control-Allow-Origin"))
		}
		if p.reached {
			t.Fatal("route ran for a preflight")
		}
	})

	t.Run("CORS refusal happens outside audit", func(t *testing.T) {
		p := newMiddlewarePin(t, false)
		rec := p.serve(http.MethodPost, "/pin/ok", http.Header{"Origin": {crossOrigin}})
		if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "forbidden origin") {
			t.Fatalf("cross-origin write = %d %q, want CORS refusal", rec.Code, rec.Body.String())
		}
		if p.reached {
			t.Fatal("route ran for a cross-origin write")
		}
		if events := p.auditEvents(); len(events) != 0 {
			t.Fatalf("audit logged a CORS refusal: %+v (audit must be inside CORS)", events)
		}
	})

	t.Run("audit records the route status through gzip", func(t *testing.T) {
		p := newMiddlewarePin(t, false)
		rec := p.serve(http.MethodPost, "/pin/teapot", http.Header{"Accept-Encoding": {"gzip"}})
		if rec.Code != http.StatusTeapot || rec.Header().Get("Content-Encoding") != "gzip" {
			t.Fatalf("teapot = %d encoding=%q, want 418 gzip", rec.Code, rec.Header().Get("Content-Encoding"))
		}
		events := p.auditEvents()
		if len(events) != 1 || events[0].Details["status"] != http.StatusTeapot || events[0].Target != "POST /pin/teapot" {
			t.Fatalf("audit events = %+v, want one POST /pin/teapot with status 418", events)
		}
	})

	t.Run("gzip compresses an accepted answer", func(t *testing.T) {
		p := newMiddlewarePin(t, false)
		rec := p.serve(http.MethodGet, "/pin/ok", http.Header{"Accept-Encoding": {"gzip"}})
		if rec.Code != http.StatusOK || rec.Header().Get("Content-Encoding") != "gzip" {
			t.Fatalf("GET = %d encoding=%q, want 200 gzip", rec.Code, rec.Header().Get("Content-Encoding"))
		}
		zr, err := gzip.NewReader(bytes.NewReader(rec.Body.Bytes()))
		if err != nil {
			t.Fatalf("gzip reader: %v", err)
		}
		plain, err := io.ReadAll(zr)
		if err != nil || string(plain) != middlewarePinBody {
			t.Fatalf("decompressed body = %q (%v), want %q", plain, err, middlewarePinBody)
		}
	})

	t.Run("every real route is wrapped", func(t *testing.T) {
		sut := newTestAPIServer(t)
		raw := sut.buildHTTPHandlers(nil, nil, nil)
		want := sortedRouteKeys(raw)
		wrapped := wrapHTTPHandlers(sut, raw)
		if got := sortedRouteKeys(wrapped); strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Fatalf("wrapHTTPHandlers changed the route keys")
		}
		req := httptest.NewRequest(http.MethodGet, "/api/v2/openapi.json", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		rec := httptest.NewRecorder()
		wrapped["/api/v2/openapi.json"](rec, req)
		if rec.Code != http.StatusOK || rec.Header().Get("Content-Encoding") != "gzip" {
			t.Fatalf("wrapped openapi = %d encoding=%q, want 200 gzip", rec.Code, rec.Header().Get("Content-Encoding"))
		}
	})
}

const middlewarePinBody = `{"probe":"ok"}`

type middlewarePin struct {
	t        *testing.T
	sut      *apiServer
	handlers map[string]http.HandlerFunc
	reached  bool
}

func newMiddlewarePin(t *testing.T, demo bool) *middlewarePin {
	t.Helper()
	p := &middlewarePin{t: t, sut: newTestAPIServer(t)}
	p.sut.demoMode = demo
	p.sut.auditEventCh = make(chan audit.Event, 16)
	probes := map[string]http.HandlerFunc{
		"/pin/ok": func(w http.ResponseWriter, _ *http.Request) {
			p.reached = true
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, middlewarePinBody)
		},
		"/pin/teapot": func(w http.ResponseWriter, _ *http.Request) {
			p.reached = true
			w.WriteHeader(http.StatusTeapot)
			_, _ = io.WriteString(w, middlewarePinBody)
		},
		"/pin/panic": func(http.ResponseWriter, *http.Request) {
			p.reached = true
			panic("wiring pin probe panic")
		},
	}
	p.handlers = wrapHTTPHandlers(p.sut, probes)
	return p
}

func (p *middlewarePin) serve(method, path string, headers http.Header) *httptest.ResponseRecorder {
	p.t.Helper()
	req := httptest.NewRequest(method, path, nil)
	for name, values := range headers {
		for _, value := range values {
			req.Header.Add(name, value)
		}
	}
	rec := httptest.NewRecorder()
	p.handlers[path](rec, req)
	return rec
}

func (p *middlewarePin) auditEvents() []audit.Event {
	var events []audit.Event
	for {
		select {
		case event := <-p.sut.auditEventCh:
			events = append(events, event)
		default:
			return events
		}
	}
}

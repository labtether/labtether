package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDemoMiddleware_AllowsGET(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := demoReadOnlyMiddleware(inner)
	req := httptest.NewRequest("GET", "/api/v2/assets", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET should be allowed, got %d", rec.Code)
	}
}

func TestDemoMiddleware_AllowsHEAD(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	handler := demoReadOnlyMiddleware(inner)
	req := httptest.NewRequest("HEAD", "/api/v2/assets", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("HEAD should be allowed, got %d", rec.Code)
	}
}

func TestDemoMiddleware_AllowsOPTIONS(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	handler := demoReadOnlyMiddleware(inner)
	req := httptest.NewRequest("OPTIONS", "/api/v2/assets", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("OPTIONS should be allowed, got %d", rec.Code)
	}
}

func TestDemoMiddleware_BlocksPOST(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	handler := demoReadOnlyMiddleware(inner)
	req := httptest.NewRequest("POST", "/api/v2/assets/srv1/exec", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("POST should be blocked, got %d", rec.Code)
	}
}

func TestDemoMiddleware_BlocksPUT(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	handler := demoReadOnlyMiddleware(inner)
	req := httptest.NewRequest("PUT", "/api/v2/alerts/rule1", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("PUT should be blocked, got %d", rec.Code)
	}
}

func TestDemoMiddleware_BlocksDELETE(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	handler := demoReadOnlyMiddleware(inner)
	req := httptest.NewRequest("DELETE", "/api/v2/assets/srv1", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("DELETE should be blocked, got %d", rec.Code)
	}
}

func TestDemoMiddleware_AllowsDemoSessionPOST(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	handler := demoReadOnlyMiddleware(inner)
	req := httptest.NewRequest("POST", "/api/demo/session", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/demo/session should be allowed, got %d", rec.Code)
	}
}

func TestDemoMiddleware_BlocksPATCH(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	handler := demoReadOnlyMiddleware(inner)
	req := httptest.NewRequest("PATCH", "/api/v2/assets/srv1", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("PATCH should be blocked, got %d", rec.Code)
	}
}

// The iPhone app asks for an event ticket before it opens the live stream, and
// signs out with a POST. In demo mode both must pass, or the app shows
// "Disconnected" and leaves the session open; neither changes shared data.
func TestDemoMiddleware_AllowsAppSessionPOSTs(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	handler := demoReadOnlyMiddleware(next)
	for _, path := range []string{"/ws/events/ticket", "/auth/logout", "/api/auth/logout"} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("POST %s should be allowed in demo mode, got %d", path, rec.Code)
		}
	}
	for _, path := range []string{"/ws/events/ticket/extra", "/auth/2fa/setup", "/api/v1/devices/register"} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, nil))
		if rec.Code != http.StatusForbidden {
			t.Errorf("POST %s should be refused in demo mode, got %d", path, rec.Code)
		}
	}
}

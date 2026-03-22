package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleHubRootReturnsLandingPayload(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.tlsState.Enabled = true

	req := httptest.NewRequest(http.MethodGet, "https://127.0.0.1:8443/", nil)
	rec := httptest.NewRecorder()

	sut.handleHubRoot(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode payload: %v", err)
	}
	if got := payload["service"]; got != "labtether-hub" {
		t.Fatalf("service=%v want=%q", got, "labtether-hub")
	}
	if got := payload["console_url"]; got != "https://127.0.0.1:3000" {
		t.Fatalf("console_url=%v want=%q", got, "https://127.0.0.1:3000")
	}
	if got := payload["healthz_path"]; got != "/healthz" {
		t.Fatalf("healthz_path=%v want=%q", got, "/healthz")
	}
}

func TestHandleHubRootRespectsConsoleURLEnvOverride(t *testing.T) {
	t.Setenv("LABTETHER_CONSOLE_URL", "https://console.example.com")
	sut := newTestAPIServer(t)

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8080/", nil)
	rec := httptest.NewRecorder()

	sut.handleHubRoot(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode payload: %v", err)
	}
	if got := payload["console_url"]; got != "https://console.example.com" {
		t.Fatalf("console_url=%v want=%q", got, "https://console.example.com")
	}
}

func TestHandleHubRootRejectsNonRootPath(t *testing.T) {
	sut := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodGet, "/not-found", nil)
	rec := httptest.NewRecorder()

	sut.handleHubRoot(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d want=%d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleHubRootRejectsUnsupportedMethod(t *testing.T) {
	sut := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()

	sut.handleHubRoot(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d want=%d", rec.Code, http.StatusMethodNotAllowed)
	}
}

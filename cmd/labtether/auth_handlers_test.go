package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/auth"
)

func TestHandleAuthLoginReturnsServiceUnavailableWithoutAuthStore(t *testing.T) {
	var sut apiServer

	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(`{"username":"admin","password":"password"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	sut.handleAuthLogin(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "An internal error occurred.") {
		t.Fatalf("expected sanitized error message, got %q", rr.Body.String())
	}
}

func TestHandleAuthLogoutClearsCookieWhenAuthStoreUnavailable(t *testing.T) {
	var sut apiServer

	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	req.AddCookie(&http.Cookie{
		Name:  auth.SessionCookieName,
		Value: "stale-token",
		Path:  "/",
	})
	rr := httptest.NewRecorder()

	sut.handleAuthLogout(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	setCookie := rr.Header().Get("Set-Cookie")
	if !strings.Contains(setCookie, auth.SessionCookieName+"=") {
		t.Fatalf("expected session cookie clear header, got %q", setCookie)
	}
}

func TestHandleAuthProvidersDefaultsToLocalOnly(t *testing.T) {
	var sut apiServer
	req := httptest.NewRequest(http.MethodGet, "/auth/providers", nil)
	rec := httptest.NewRecorder()

	sut.handleAuthProviders(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var payload struct {
		Local struct {
			Enabled bool `json:"enabled"`
		} `json:"local"`
		OIDC struct {
			Enabled bool `json:"enabled"`
		} `json:"oidc"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Local.Enabled {
		t.Fatalf("expected local auth provider to be enabled")
	}
	if payload.OIDC.Enabled {
		t.Fatalf("expected oidc provider to be disabled by default")
	}
}

func TestHandleAuthUsersReturnsServiceUnavailableWithoutAuthStore(t *testing.T) {
	var sut apiServer
	req := httptest.NewRequest(http.MethodGet, "/auth/users", nil)
	rec := httptest.NewRecorder()

	sut.handleAuthUsers(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "An internal error occurred.") {
		t.Fatalf("expected sanitized error message, got %q", rec.Body.String())
	}
}

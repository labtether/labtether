package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSetSessionCookieSecureOverHTTPS(t *testing.T) {
	rec := httptest.NewRecorder()

	SetSessionCookie(rec, "raw-token", 30*time.Minute, true)

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected 1 cookie, got %d", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != SessionCookieName {
		t.Fatalf("expected cookie %q, got %q", SessionCookieName, cookie.Name)
	}
	if cookie.Value != "raw-token" {
		t.Fatalf("expected raw token value, got %q", cookie.Value)
	}
	if cookie.Path != "/" {
		t.Fatalf("expected root path, got %q", cookie.Path)
	}
	if cookie.MaxAge != int((30 * time.Minute).Seconds()) {
		t.Fatalf("expected 30-minute max age, got %d", cookie.MaxAge)
	}
	if !cookie.HttpOnly {
		t.Fatal("expected HttpOnly session cookie")
	}
	if !cookie.Secure {
		t.Fatal("expected Secure session cookie")
	}
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("expected SameSite=Lax, got %v", cookie.SameSite)
	}
}

// A Secure cookie set over plain HTTP is dropped by browsers, so plain-HTTP
// sign-in from another machine must get a cookie without Secure.
func TestSetSessionCookieOverPlainHTTPIsNotSecure(t *testing.T) {
	rec := httptest.NewRecorder()
	SetSessionCookie(rec, "raw-token", 30*time.Minute, false)
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Secure || !cookies[0].HttpOnly {
		t.Fatalf("plain-HTTP session cookie = %+v, want HttpOnly without Secure", cookies)
	}
}

func TestClearSessionCookieExpiresCookie(t *testing.T) {
	rec := httptest.NewRecorder()

	ClearSessionCookie(rec)

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected 1 cookie, got %d", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != SessionCookieName {
		t.Fatalf("expected cookie %q, got %q", SessionCookieName, cookie.Name)
	}
	if cookie.Value != "" {
		t.Fatalf("expected empty cookie value, got %q", cookie.Value)
	}
	if cookie.Path != "/" {
		t.Fatalf("expected root path, got %q", cookie.Path)
	}
	if cookie.MaxAge >= 0 {
		t.Fatalf("expected clearing cookie max age, got %d", cookie.MaxAge)
	}
	if !cookie.HttpOnly {
		t.Fatal("expected HttpOnly session cookie")
	}
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("expected SameSite=Lax, got %v", cookie.SameSite)
	}
}

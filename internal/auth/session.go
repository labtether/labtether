package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	SessionCookieName = "labtether_session"
	SessionDuration   = 24 * time.Hour
)

func GenerateSessionToken() (raw string, hashed string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("generate session token: %w", err)
	}
	raw = base64.RawURLEncoding.EncodeToString(b)
	hash := sha256.Sum256([]byte(raw))
	hashed = base64.RawURLEncoding.EncodeToString(hash[:])
	return raw, hashed, nil
}

func HashToken(raw string) string {
	hash := sha256.Sum256([]byte(raw))
	return base64.RawURLEncoding.EncodeToString(hash[:])
}

func SetSessionCookie(w http.ResponseWriter, token string, maxAge time.Duration, secure bool) {
	http.SetCookie(w, &http.Cookie{ // #nosec G124 -- Secure is intentionally caller-controlled for local HTTP mode; HttpOnly and SameSite are always enforced.
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(maxAge.Seconds()),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func ClearSessionCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{ // #nosec G124 -- Secure is intentionally caller-controlled for local HTTP mode; HttpOnly and SameSite are always enforced.
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func ExtractSessionToken(r *http.Request) string {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil || cookie == nil {
		return ""
	}
	return strings.TrimSpace(cookie.Value)
}

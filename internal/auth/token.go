package auth

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// TokenValidator performs constant-time token validation for API requests.
type TokenValidator struct {
	tokens []string
}

func NewTokenValidator(tokens ...string) *TokenValidator {
	normalized := make([]string, 0, len(tokens))
	seen := make(map[string]struct{}, len(tokens))
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		normalized = append(normalized, token)
	}
	return &TokenValidator{tokens: normalized}
}

func (v *TokenValidator) Configured() bool {
	return v != nil && len(v.tokens) > 0
}

func (v *TokenValidator) ValidateRequest(r *http.Request) bool {
	if !v.Configured() {
		return false
	}
	provided := extractToken(r)
	if provided == "" {
		return false
	}
	for _, token := range v.tokens {
		if len(provided) != len(token) {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(provided), []byte(token)) == 1 {
			return true
		}
	}
	return false
}

// ExtractBearerToken extracts a bearer or X-Labtether-Token from the request.
func ExtractBearerToken(r *http.Request) string {
	return extractToken(r)
}

func extractToken(r *http.Request) string {
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			return strings.TrimSpace(parts[1])
		}
	}

	return strings.TrimSpace(r.Header.Get("X-Labtether-Token"))
}

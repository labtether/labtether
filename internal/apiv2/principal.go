package apiv2

import (
	"context"
	"strings"
)

// IsAPIKeyPrincipal reports whether ctx represents an API-key-authenticated
// principal. The explicit key ID is authoritative; the actor-prefix fallback
// keeps legacy/custom middleware from accidentally losing this distinction.
func IsAPIKeyPrincipal(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	if strings.TrimSpace(APIKeyIDFromContext(ctx)) != "" {
		return true
	}
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(UserIDFromContext(ctx))), "apikey:")
}

// IsOwnerPrincipal reports whether ctx carries the reserved interactive owner
// role. Owner authority is role-based because persisted users have generated
// IDs; it must never be inferred from an actor ID such as "owner". API keys
// are categorically excluded even if a legacy or corrupt row carries the
// reserved owner role.
func IsOwnerPrincipal(ctx context.Context) bool {
	if ctx == nil || IsAPIKeyPrincipal(ctx) {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(UserRoleFromContext(ctx)), "owner")
}

package apiv2

import (
	"context"
	"strings"
)

// IsOwnerPrincipal reports whether ctx carries the reserved interactive owner
// role. Owner authority is role-based because persisted users have generated
// IDs; it must never be inferred from an actor ID such as "owner". API keys
// are categorically excluded even if a legacy or corrupt row carries the
// reserved owner role.
func IsOwnerPrincipal(ctx context.Context) bool {
	if ctx == nil || strings.TrimSpace(APIKeyIDFromContext(ctx)) != "" {
		return false
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(UserIDFromContext(ctx))), "apikey:") {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(UserRoleFromContext(ctx)), "owner")
}

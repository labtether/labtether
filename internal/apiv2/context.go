package apiv2

import (
	"context"
	"strings"
)

// contextKey is an unexported type for all context keys in this package.
// Using a package-local type prevents collisions with keys from other packages.
type contextKey string

const (
	userIDContextKey       contextKey = "user_id"
	userRoleContextKey     contextKey = "user_role"
	scopesContextKey       contextKey = "api_scopes"
	allowedAssetsContextKey contextKey = "api_allowed_assets"
	apiKeyIDContextKey     contextKey = "api_key_id"
)

// ContextWithUserID returns a new context carrying the authenticated user ID.
func ContextWithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDContextKey, userID)
}

// ContextWithUserRole returns a new context carrying the authenticated user role.
func ContextWithUserRole(ctx context.Context, role string) context.Context {
	return context.WithValue(ctx, userRoleContextKey, role)
}

// ContextWithPrincipal returns a new context carrying both user ID and role.
// It is a convenience wrapper around ContextWithUserID and ContextWithUserRole.
func ContextWithPrincipal(ctx context.Context, userID, role string) context.Context {
	ctx = ContextWithUserID(ctx, userID)
	return ContextWithUserRole(ctx, role)
}

// UserIDFromContext retrieves the user ID stored by ContextWithUserID.
// Returns an empty string if no user ID is present.
func UserIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(userIDContextKey).(string); ok {
		return v
	}
	return ""
}

// UserRoleFromContext retrieves the user role stored by ContextWithUserRole.
// Returns an empty string if no role is present.
func UserRoleFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(userRoleContextKey).(string); ok {
		return v
	}
	return ""
}

// PrincipalActorID returns the actor identifier for audit-log attribution.
// When no user ID is present (e.g. unauthenticated background work), it
// returns the sentinel value "system".
func PrincipalActorID(ctx context.Context) string {
	actorID := strings.TrimSpace(UserIDFromContext(ctx))
	if actorID == "" {
		return "system"
	}
	return actorID
}

// ContextWithScopes returns a new context carrying the API key scopes.
// If ctx is nil, context.Background() is used.
func ContextWithScopes(ctx context.Context, scopes []string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, scopesContextKey, scopes)
}

// ScopesFromContext retrieves the API key scopes stored by ContextWithScopes.
// Returns nil if no scopes are present.
func ScopesFromContext(ctx context.Context) []string {
	if v, ok := ctx.Value(scopesContextKey).([]string); ok {
		return v
	}
	return nil
}

// ContextWithAllowedAssets returns a new context carrying the API key asset allowlist.
// If ctx is nil, context.Background() is used.
func ContextWithAllowedAssets(ctx context.Context, assets []string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, allowedAssetsContextKey, assets)
}

// AllowedAssetsFromContext retrieves the asset allowlist stored by ContextWithAllowedAssets.
// Returns nil if no allowlist is present.
func AllowedAssetsFromContext(ctx context.Context) []string {
	if v, ok := ctx.Value(allowedAssetsContextKey).([]string); ok {
		return v
	}
	return nil
}

// ContextWithAPIKeyID returns a new context carrying the API key ID.
// If ctx is nil, context.Background() is used.
func ContextWithAPIKeyID(ctx context.Context, keyID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, apiKeyIDContextKey, keyID)
}

// APIKeyIDFromContext retrieves the API key ID stored by ContextWithAPIKeyID.
// Returns an empty string if no key ID is present.
func APIKeyIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(apiKeyIDContextKey).(string); ok {
		return v
	}
	return ""
}

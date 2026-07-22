package apiv2

import (
	"context"
	"net/http"

	"github.com/labtether/labtether/internal/apikeys"
)

// ScopeCheck checks if the granted scopes allow the required scope.
// Returns true if no scopes are present (session/owner auth — full access)
// or if the required scope is granted.
func ScopeCheck(grantedScopes []string, required string) bool {
	if grantedScopes == nil {
		return true
	}
	return apikeys.ScopeAllows(grantedScopes, required)
}

// AssetCheck returns true if the asset is accessible given the allowlist.
// Nil/empty allowlist means all assets are accessible.
func AssetCheck(allowedAssets []string, assetID string) bool {
	return apikeys.AssetAllowed(allowedAssets, assetID)
}

// AssetCheckContext applies the authenticated API key's asset allowlist to an
// asset identifier. Session and owner authentication have no allowlist in the
// context and therefore retain full access.
func AssetCheckContext(ctx context.Context, assetID string) bool {
	return AssetCheck(AllowedAssetsFromContext(ctx), assetID)
}

// RequireAssetAccess enforces object-level asset authorization consistently
// across handlers whose asset identifier is not encoded in a standard route.
func RequireAssetAccess(w http.ResponseWriter, r *http.Request, assetID string) bool {
	if r != nil && AssetCheckContext(r.Context(), assetID) {
		return true
	}
	WriteAssetForbidden(w, assetID)
	return false
}

// RequireScope enforces an API-key scope while preserving full access for
// cookie sessions and the owner bearer token (which carry nil scopes).
func RequireScope(w http.ResponseWriter, r *http.Request, required string) bool {
	if r != nil && ScopeCheck(ScopesFromContext(r.Context()), required) {
		return true
	}
	WriteScopeForbidden(w, required)
	return false
}

// IsMutatingMethod returns true if the HTTP method modifies state.
// GET, HEAD, and OPTIONS are considered safe (read-only).
func IsMutatingMethod(method string) bool {
	return method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions
}

// WriteScopeForbidden writes a 403 for insufficient scope.
func WriteScopeForbidden(w http.ResponseWriter, required string) {
	WriteError(w, http.StatusForbidden, "insufficient_scope",
		"api key lacks required scope: "+required)
}

// WriteAssetForbidden writes a 403 for an inaccessible asset.
func WriteAssetForbidden(w http.ResponseWriter, assetID string) {
	WriteError(w, http.StatusForbidden, "asset_forbidden",
		"api key does not have access to asset: "+assetID)
}

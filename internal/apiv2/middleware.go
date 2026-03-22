package apiv2

import (
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

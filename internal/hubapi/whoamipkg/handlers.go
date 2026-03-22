package whoamipkg

import (
	"context"
	"net/http"

	"github.com/labtether/labtether/internal/apiv2"
)

// HandleV2Whoami handles GET /api/v2/whoami.
// It returns the authenticated principal's identity, role, scopes, and
// (for API-key sessions) the list of accessible assets.
func (d *Deps) HandleV2Whoami(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apiv2.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}

	ctx := r.Context()
	keyID := apiv2.APIKeyIDFromContext(ctx)
	role := apiv2.UserRoleFromContext(ctx)
	userID := apiv2.UserIDFromContext(ctx)

	result := map[string]any{
		"user_id": userID,
		"role":    role,
	}

	if keyID != "" {
		result["auth_type"] = "api_key"
		result["key_id"] = keyID
		result["scopes"] = apiv2.ScopesFromContext(ctx)
		result["allowed_assets"] = apiv2.AllowedAssetsFromContext(ctx)

		if d.APIKeyStore != nil {
			key, ok, err := d.APIKeyStore.GetAPIKey(r.Context(), keyID)
			if err == nil && ok {
				result["key_name"] = key.Name
				result["expires_at"] = key.ExpiresAt
			}
		}

		result["available_assets"] = d.listAccessibleAssets(ctx)
	} else {
		result["auth_type"] = "session"
		result["scopes"] = []string{"*"}
	}

	apiv2.WriteJSON(w, http.StatusOK, result)
}

// listAccessibleAssets returns the assets accessible to the current API key
// principal, filtered by the allowedAssets set from the request context.
func (d *Deps) listAccessibleAssets(ctx context.Context) []map[string]any {
	if d.AssetStore == nil {
		return nil
	}
	allAssets, err := d.AssetStore.ListAssets()
	if err != nil {
		return nil
	}

	allowed := apiv2.AllowedAssetsFromContext(ctx)
	result := make([]map[string]any, 0)
	for _, a := range allAssets {
		if !apiv2.AssetCheck(allowed, a.ID) {
			continue
		}
		online := a.Status == "online"
		entry := map[string]any{
			"id":       a.ID,
			"name":     a.Name,
			"platform": a.Platform,
			"status":   a.Status,
			"online":   online,
		}
		if !online {
			entry["last_seen"] = a.LastSeenAt
		}
		result = append(result, entry)
	}
	return result
}

package auth

import (
	"net/http"

	"github.com/labtether/labtether/internal/apiv2"
)

// rejectAPIKeyIdentityMutation keeps scoped API credentials from changing the
// identities or credentials that define their own authority. Cookie sessions
// and the owner bearer are intentionally unaffected.
func rejectAPIKeyIdentityMutation(w http.ResponseWriter, r *http.Request) bool {
	if r == nil || !apiv2.IsAPIKeyPrincipal(r.Context()) {
		return false
	}
	apiv2.WriteError(w, http.StatusForbidden, "api_key_forbidden", "API keys cannot modify identity administration")
	return true
}

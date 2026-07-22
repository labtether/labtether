package main

import (
	"net/http"
	"strings"

	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/servicehttp"
)

// withAgentHeartbeatAuth admits a per-agent token only to the exact legacy
// heartbeat endpoint. The token's asset binding is carried in request context
// so the heartbeat handler can reject attempts to update any other asset.
// All other credentials and routes retain the normal withAuth behavior.
func (s *apiServer) withAgentHeartbeatAuth(next http.HandlerFunc) http.HandlerFunc {
	defaultAuth := s.withAuth(func(w http.ResponseWriter, r *http.Request) {
		ctx := shared.ContextWithExistingAgentHeartbeatOnly(r.Context())
		if !s.allowLegacySharedAgentAuth {
			ctx = shared.ContextWithSharedAgentHeartbeatDisabled(ctx)
		}
		next(w, r.WithContext(ctx))
	})
	return func(w http.ResponseWriter, r *http.Request) {
		if r == nil || r.Method != http.MethodPost || r.URL.Path != "/assets/heartbeat" {
			defaultAuth(w, r)
			return
		}

		bearer := auth.ExtractBearerToken(r)
		if bearer == "" || s.enrollmentStore == nil {
			defaultAuth(w, r)
			return
		}

		agentToken, valid, err := s.enrollmentStore.ValidateAgentToken(auth.HashToken(bearer))
		if err != nil {
			servicehttp.WriteError(w, http.StatusServiceUnavailable, "agent token lookup failed")
			return
		}
		if !valid {
			defaultAuth(w, r)
			return
		}

		assetID := strings.TrimSpace(agentToken.AssetID)
		if assetID == "" {
			servicehttp.WriteError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if !s.apiKeyRequestIsSecure(r) {
			servicehttp.WriteError(w, http.StatusForbidden, "agent tokens require HTTPS")
			return
		}

		ctx := contextWithPrincipal(r.Context(), "agent:"+assetID, auth.RoleOperator)
		ctx = contextWithScopes(ctx, []string{"assets:write"})
		ctx = contextWithAllowedAssets(ctx, []string{assetID})
		ctx = shared.ContextWithAgentTokenID(ctx, agentToken.ID)
		next(w, r.WithContext(ctx))
	}
}

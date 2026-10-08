package main

import (
	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/audit"
	"net/http"
	"time"
)

// --- Hub Status ---

func (s *apiServer) handleV2HubStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apiv2.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET required")
		return
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "hub:read") {
		apiv2.WriteScopeForbidden(w, "hub:read")
		return
	}
	agentCount := 0
	if s.agentMgr != nil {
		if len(allowedAssetsFromContext(r.Context())) == 0 {
			agentCount = s.agentMgr.Count()
		} else {
			for _, assetID := range s.agentMgr.ConnectedAssets() {
				if apiv2.AssetCheckContext(r.Context(), assetID) {
					agentCount++
				}
			}
		}
	}
	apiv2.WriteJSON(w, http.StatusOK, map[string]any{
		"status":           "running",
		"agents_connected": agentCount,
		"demo":             s.demoMode,
	})
}

func (s *apiServer) handleV2HubAgents(w http.ResponseWriter, r *http.Request) {
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "hub:read") {
		apiv2.WriteScopeForbidden(w, "hub:read")
		return
	}
	r.URL.Path = "/agents/presence"
	apiv2.WrapV1Handler(s.handleAgentPresence)(w, r)
}

func (s *apiServer) handleV2HubTLS(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminAuth(w, r) {
		return
	}
	if denyAssetRestrictedGlobalAPI(w, r, "hub TLS settings") {
		return
	}
	scope := "hub:read"
	if apiv2.IsMutatingMethod(r.Method) {
		scope = "hub:admin"
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	r.URL.Path = "/settings/tls"
	apiv2.WrapV1Handler(s.handleTLSSettings)(w, r)
}

func (s *apiServer) handleV2HubTailscale(w http.ResponseWriter, r *http.Request) {
	if denyAssetRestrictedGlobalAPI(w, r, "hub Tailscale settings") {
		return
	}
	scope := "hub:read"
	if apiv2.IsMutatingMethod(r.Method) {
		scope = "hub:admin"
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	r.URL.Path = "/settings/tailscale/serve"
	apiv2.WrapV1Handler(s.handleTailscaleServeStatus)(w, r)
}

// --- TLS Renew ---

// handleV2HubTLSRenew handles POST /api/v2/hub/tls/renew.
//
// Behaviour by TLS source:
//   - built-in (self-signed):  forces immediate renewal via certmgr.CertReloader.
//   - tailscale:               re-runs `tailscale cert` via the tailscale reloader.
//   - uploaded / external:     returns 422 — those certs cannot be renewed
//     by the hub automatically.
//   - TLS disabled:            returns 422.
func (s *apiServer) handleV2HubTLSRenew(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminAuth(w, r) {
		return
	}
	if denyAssetRestrictedGlobalAPI(w, r, "hub TLS renewal") {
		return
	}
	if r.Method != http.MethodPost {
		apiv2.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST required")
		return
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "hub:admin") {
		apiv2.WriteScopeForbidden(w, "hub:admin")
		return
	}

	switch s.tlsState.Source {
	case tlsSourceBuiltIn:
		if s.tlsState.CertReloader == nil {
			apiv2.WriteError(w, http.StatusServiceUnavailable, "unavailable", "built-in cert reloader is not initialised")
			return
		}
		if err := s.tlsState.CertReloader.ForceRenew(); err != nil {
			apiv2.WriteError(w, http.StatusInternalServerError, "renewal_failed", "failed to renew built-in certificate: "+err.Error())
			return
		}
		s.appendAuditEventBestEffort(audit.Event{
			Type:      "hub.tls.renewed",
			ActorID:   principalActorID(r.Context()),
			Details:   map[string]any{"tls_source": tlsSourceBuiltIn},
			Timestamp: time.Now().UTC(),
		}, "v2 hub tls renew (built-in)")
		apiv2.WriteJSON(w, http.StatusOK, map[string]string{"status": "renewed", "tls_source": tlsSourceBuiltIn})

	case tlsSourceTailscale:
		tsrl, tsrlOK := s.tlsState.TailscaleCertReloader.(*tailscaleCertReloader)
		if !tsrlOK || tsrl == nil {
			apiv2.WriteError(w, http.StatusServiceUnavailable, "unavailable", "tailscale cert reloader is not initialised")
			return
		}
		if err := tsrl.renew(); err != nil {
			apiv2.WriteError(w, http.StatusInternalServerError, "renewal_failed", "failed to renew tailscale certificate: "+err.Error())
			return
		}
		s.appendAuditEventBestEffort(audit.Event{
			Type:      "hub.tls.renewed",
			ActorID:   principalActorID(r.Context()),
			Details:   map[string]any{"tls_source": tlsSourceTailscale},
			Timestamp: time.Now().UTC(),
		}, "v2 hub tls renew (tailscale)")
		apiv2.WriteJSON(w, http.StatusOK, map[string]string{"status": "renewed", "tls_source": tlsSourceTailscale})

	case tlsSourceUIUploaded, tlsSourceDeploymentExternal:
		apiv2.WriteError(w, http.StatusUnprocessableEntity, "unsupported_source",
			"certificate renewal is not supported for source '"+s.tlsState.Source+"'; upload a new certificate via POST /api/v2/hub/tls")

	default:
		apiv2.WriteError(w, http.StatusUnprocessableEntity, "tls_disabled",
			"TLS is not enabled on this hub; renewal is not applicable")
	}
}

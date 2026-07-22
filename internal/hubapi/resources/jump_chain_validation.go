package resources

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/servicehttp"
	terminalcfg "github.com/labtether/labtether/internal/terminal"
)

func (d *Deps) normalizeGroupJumpChain(w http.ResponseWriter, r *http.Request, raw json.RawMessage) (json.RawMessage, bool) {
	chain, err := terminalcfg.DecodeJumpChain(raw)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return nil, false
	}
	if terminalcfg.JumpChainUsesCredential(chain) {
		if !requireAPIScope(w, r, "credentials:use") {
			return nil, false
		}
		if d.CredentialStore == nil {
			servicehttp.WriteError(w, http.StatusServiceUnavailable, "credential store unavailable")
			return nil, false
		}
		for _, hop := range chain.Hops {
			profileID := strings.TrimSpace(hop.CredentialProfileID)
			profile, ok, lookupErr := d.CredentialStore.GetCredentialProfile(profileID)
			if lookupErr != nil {
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to validate jump-chain credential")
				return nil, false
			}
			if !ok {
				servicehttp.WriteError(w, http.StatusBadRequest, "jump-chain credential profile not found")
				return nil, false
			}
			switch strings.TrimSpace(profile.Kind) {
			case credentials.KindSSHPassword, credentials.KindSSHPrivateKey, credentials.KindHubSSHIdentity:
			default:
				servicehttp.WriteError(w, http.StatusBadRequest, "jump-chain credential profile is not SSH-compatible")
				return nil, false
			}
		}
	}
	normalized, err := json.Marshal(chain)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to normalize jump chain")
		return nil, false
	}
	return normalized, true
}

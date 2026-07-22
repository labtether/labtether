package terminal

import (
	"context"
	"net/http"
	"strings"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/protocols"
	"github.com/labtether/labtether/internal/servicehttp"
	"github.com/labtether/labtether/internal/terminal"
)

func (d *Deps) requireTargetAccess(w http.ResponseWriter, r *http.Request, target string) bool {
	return apiv2.RequireAssetAccess(w, r, strings.TrimSpace(target))
}

func (d *Deps) targetUsesStoredCredential(ctx context.Context, target string) (bool, error) {
	target = strings.TrimSpace(target)
	if d.GetProtocolConfig != nil {
		for _, protocol := range []string{protocols.ProtocolSSH, protocols.ProtocolTelnet} {
			cfg, err := d.GetProtocolConfig(ctx, target, protocol)
			if err != nil {
				return false, err
			}
			if cfg != nil && strings.TrimSpace(cfg.CredentialProfileID) != "" {
				return true, nil
			}
		}
	}
	// The terminal-config bridge can be present even when the backing credential
	// store is disabled (for example in reduced test/runtime configurations).
	// Only consult it when the store is actually available.
	if d.CredentialStore != nil && d.GetAssetTerminalConfig != nil {
		cfg, ok, err := d.GetAssetTerminalConfig(target)
		if err != nil {
			return false, err
		}
		if ok && strings.TrimSpace(cfg.CredentialProfileID) != "" {
			return true, nil
		}
	}
	if d.AssetStore != nil && d.GroupStore != nil {
		asset, ok, err := d.AssetStore.GetAsset(target)
		if err != nil {
			return false, err
		}
		if ok && strings.TrimSpace(asset.GroupID) != "" {
			group, found, err := d.GroupStore.GetGroup(strings.TrimSpace(asset.GroupID))
			if err != nil {
				return false, err
			}
			if found {
				chain, err := terminal.DecodeJumpChain(group.JumpChain)
				if err != nil {
					return false, err
				}
				if terminal.JumpChainUsesCredential(chain) {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

func (d *Deps) requireCredentialUseForTarget(w http.ResponseWriter, r *http.Request, target string) bool {
	usesCredential, err := d.targetUsesStoredCredential(r.Context(), target)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to authorize stored credential use")
		return false
	}
	if !usesCredential {
		return true
	}
	return apiv2.RequireScope(w, r, "credentials:use")
}

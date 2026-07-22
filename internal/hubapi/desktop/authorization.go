package desktop

import (
	"context"
	"net/http"
	"strings"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/protocols"
	"github.com/labtether/labtether/internal/servicehttp"
)

func (d *Deps) requireTargetAccess(w http.ResponseWriter, r *http.Request, target string) bool {
	return apiv2.RequireAssetAccess(w, r, strings.TrimSpace(target))
}

func (d *Deps) targetUsesStoredCredential(ctx context.Context, target string) (bool, error) {
	target = strings.TrimSpace(target)
	if d.GetProtocolConfig != nil {
		for _, protocol := range []string{protocols.ProtocolRDP, protocols.ProtocolVNC, protocols.ProtocolARD} {
			cfg, err := d.GetProtocolConfig(ctx, target, protocol)
			if err != nil {
				return false, err
			}
			if cfg != nil && strings.TrimSpace(cfg.CredentialProfileID) != "" {
				return true, nil
			}
		}
	}
	// Managed RDP/VNC target resolution first consults the legacy desktop
	// configuration. Cover that profile before checking the terminal fallback so
	// an API key cannot consume a saved desktop secret without credentials:use.
	if d.CredentialStore != nil {
		desktopCfg, ok, err := d.CredentialStore.GetDesktopConfig(target)
		if err != nil {
			return false, err
		}
		if ok && strings.TrimSpace(desktopCfg.CredentialProfileID) != "" {
			return true, nil
		}

		// Desktop target resolution also falls back to the legacy per-asset
		// terminal config, so its stored profile needs the same permission check.
		cfg, ok, err := d.CredentialStore.GetAssetTerminalConfig(target)
		if err != nil {
			return false, err
		}
		if ok && strings.TrimSpace(cfg.CredentialProfileID) != "" {
			return true, nil
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

func (d *Deps) requireCredentialUseForDirectSession(w http.ResponseWriter, r *http.Request, opts DesktopSessionOptions) bool {
	if strings.TrimSpace(opts.DirectUsername) == "" && opts.DirectPassword == "" {
		return true
	}
	return apiv2.RequireScope(w, r, "credentials:use")
}

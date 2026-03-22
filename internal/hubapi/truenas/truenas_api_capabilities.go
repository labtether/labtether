package truenas

import (
	"context"
	"net/http"
	"time"

	"github.com/labtether/labtether/internal/assets"
	tnconnector "github.com/labtether/labtether/internal/connectors/truenas"
	"github.com/labtether/labtether/internal/servicehttp"
)

// TruenasCapabilities describes the tabs and features available for a TrueNAS asset.
type TruenasCapabilities struct {
	Tabs      []string `json:"tabs"`
	IsScale   bool     `json:"is_scale"` // true when vm.query succeeds (SCALE edition)
	HasApps   bool     `json:"has_apps"` // true when app.query succeeds (SCALE apps)
	Warnings  []string `json:"warnings,omitempty"`
	FetchedAt string   `json:"fetched_at"`
}

var TruenasBaseTabs = []string{
	"overview",
	"pools",
	"datasets",
	"shares",
	"disks",
	"services",
	"snapshots",
	"replication",
	"events",
}

// handleTrueNASCapabilities handles GET /truenas/assets/{id}/capabilities.
func (d *Deps) HandleTrueNASCapabilities(ctx context.Context, w http.ResponseWriter, asset assets.Asset, runtime *TruenasRuntime) {
	caps := TruenasCapabilities{
		FetchedAt: time.Now().UTC().Format(time.RFC3339),
	}

	// Detect SCALE edition by probing vm.query. If it succeeds the instance is
	// SCALE; if IsMethodNotFound it is CORE.
	var vmResult []map[string]any
	vmErr := CallTrueNASQueryWithRetries(ctx, runtime.Client, "vm.query", &vmResult)
	switch {
	case vmErr == nil:
		caps.IsScale = true
	case tnconnector.IsMethodNotFound(vmErr):
		caps.IsScale = false
	default:
		caps.Warnings = AppendTrueNASWarning(caps.Warnings, "vm capability detection unavailable: "+vmErr.Error())
	}

	// Detect SCALE apps availability via app.query.
	if caps.IsScale {
		var appResult []map[string]any
		appErr := CallTrueNASQueryWithRetries(ctx, runtime.Client, "app.query", &appResult)
		switch {
		case appErr == nil:
			caps.HasApps = true
		case tnconnector.IsMethodNotFound(appErr):
			caps.HasApps = false
		default:
			caps.Warnings = AppendTrueNASWarning(caps.Warnings, "apps capability detection unavailable: "+appErr.Error())
		}
	}

	// Build tabs: base set plus conditional "vms" for SCALE.
	tabs := make([]string, 0, len(TruenasBaseTabs)+1)
	tabs = append(tabs, TruenasBaseTabs...)
	if caps.IsScale {
		tabs = append(tabs, "vms")
	}
	caps.Tabs = tabs

	servicehttp.WriteJSON(w, http.StatusOK, caps)
}

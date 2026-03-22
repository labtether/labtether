package truenas

import (
	"github.com/labtether/labtether/internal/hubapi/shared"
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/servicehttp"
)

type TrueNASOverviewResponse struct {
	AssetID   string                  `json:"asset_id"`
	Hostname  string                  `json:"hostname,omitempty"`
	SystemInfo map[string]any         `json:"system_info,omitempty"`
	Pools     []map[string]any        `json:"pools,omitempty"`
	Services  []TrueNASServiceEntry   `json:"services,omitempty"`
	Alerts    []map[string]any        `json:"alerts,omitempty"`
	Warnings  []string                `json:"warnings,omitempty"`
	FetchedAt string                  `json:"fetched_at"`
}

func (d *Deps) HandleTrueNASOverview(ctx context.Context, w http.ResponseWriter, r *http.Request, asset assets.Asset, runtime *TruenasRuntime) {
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	warnings := make([]string, 0, 4)

	var sysInfo map[string]any
	if err := CallTrueNASMethodWithRetries(ctx, runtime.Client, "system.info", nil, &sysInfo); err != nil {
		warnings = AppendTrueNASWarning(warnings, "system info unavailable: "+err.Error())
	}

	pools := make([]map[string]any, 0, 8)
	if err := CallTrueNASQueryWithRetries(ctx, runtime.Client, "pool.query", &pools); err != nil {
		warnings = AppendTrueNASWarning(warnings, "pools unavailable: "+err.Error())
		pools = nil
	}

	rawServices := make([]map[string]any, 0, 16)
	if err := CallTrueNASQueryWithRetries(ctx, runtime.Client, "service.query", &rawServices); err != nil {
		warnings = AppendTrueNASWarning(warnings, "services unavailable: "+err.Error())
		rawServices = nil
	}
	services := make([]TrueNASServiceEntry, 0, len(rawServices))
	for _, svc := range rawServices {
		services = append(services, MapTrueNASServiceEntry(svc))
	}

	alerts := make([]map[string]any, 0, 16)
	if err := CallTrueNASQueryWithRetries(ctx, runtime.Client, "alert.list", &alerts); err != nil {
		warnings = AppendTrueNASWarning(warnings, "alerts unavailable: "+err.Error())
		alerts = nil
	}

	hostname := strings.TrimSpace(asset.Metadata["hostname"])
	if hostname == "" && sysInfo != nil {
		hostname = strings.TrimSpace(shared.CollectorAnyString(sysInfo["hostname"]))
	}

	servicehttp.WriteJSON(w, http.StatusOK, TrueNASOverviewResponse{
		AssetID:    strings.TrimSpace(asset.ID),
		Hostname:   hostname,
		SystemInfo: sysInfo,
		Pools:      pools,
		Services:   services,
		Alerts:     alerts,
		Warnings:   warnings,
		FetchedAt:  time.Now().UTC().Format(time.RFC3339),
	})
}

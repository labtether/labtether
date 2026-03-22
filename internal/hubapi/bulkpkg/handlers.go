package bulkpkg

import (
	"net/http"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/hubapi/shared"
)

// validServiceActions is the allowlist of service management actions accepted
// by HandleV2BulkServiceAction.
var validServiceActions = map[string]bool{
	"start":   true,
	"stop":    true,
	"restart": true,
	"reload":  true,
	"enable":  true,
	"disable": true,
	"status":  true,
}

// HandleV2BulkServiceAction handles POST /api/v2/bulk/service-action.
// It runs a systemctl action against a named service on multiple target assets
// concurrently, subject to scope and asset-allowlist filtering.
func (d *Deps) HandleV2BulkServiceAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apiv2.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST required")
		return
	}
	if !apiv2.ScopeCheck(apiv2.ScopesFromContext(r.Context()), "bulk:*") {
		apiv2.WriteScopeForbidden(w, "bulk:*")
		return
	}

	var req struct {
		Action  string   `json:"action"`
		Service string   `json:"service"`
		Targets []string `json:"targets"`
	}
	if err := shared.DecodeJSONBody(w, r, &req); err != nil {
		return
	}

	if req.Action == "" || req.Service == "" || len(req.Targets) == 0 {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "action, service, and targets required")
		return
	}

	// Validate action against allowlist.
	if !validServiceActions[req.Action] {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "invalid action: must be start, stop, restart, reload, enable, disable, or status")
		return
	}

	// Validate service name — alphanumeric, dots, dashes, underscores, @ only.
	for _, c := range req.Service {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '.' || c == '-' || c == '_' || c == '@') {
			apiv2.WriteError(w, http.StatusBadRequest, "validation", "invalid service name: contains disallowed characters")
			return
		}
	}

	allowed := apiv2.AllowedAssetsFromContext(r.Context())
	command := "systemctl " + req.Action + " " + req.Service

	var filteredTargets []string
	for _, target := range req.Targets {
		if apiv2.AssetCheck(allowed, target) {
			filteredTargets = append(filteredTargets, target)
		}
	}
	if len(filteredTargets) == 0 {
		apiv2.WriteError(w, http.StatusForbidden, "asset_forbidden", "none of the requested targets are accessible with this API key")
		return
	}

	results := make(map[string]any)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, target := range filteredTargets {
		wg.Add(1)
		go func(t string) {
			defer wg.Done()
			result := d.ExecOnAsset(r, t, command, 30)
			mu.Lock()
			if result.Error != "" {
				results[t] = map[string]any{"error": result.Error, "message": result.Message}
			} else {
				results[t] = map[string]any{"status": "ok", "output": result.Stdout}
			}
			mu.Unlock()
		}(target)
	}
	wg.Wait()

	shared.AppendAuditEventBestEffort(d.AuditStore, audit.Event{
		Type:      "api.bulk.service_action",
		ActorID:   apiv2.PrincipalActorID(r.Context()),
		Details:   map[string]any{"action": req.Action, "service": req.Service, "targets": filteredTargets},
		Timestamp: time.Now().UTC(),
	}, "v2 bulk service action")

	apiv2.WriteJSON(w, http.StatusOK, map[string]any{"results": results})
}

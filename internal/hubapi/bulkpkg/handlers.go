package bulkpkg

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/hubapi/maintenanceguard"
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

const (
	maxBulkServiceRawTargets  = 64
	maxBulkServiceTargets     = 64
	maxBulkServiceConcurrency = 8
)

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
	if len(req.Targets) > maxBulkServiceRawTargets {
		apiv2.WriteError(w, http.StatusBadRequest, "validation",
			fmt.Sprintf("too many targets: maximum is %d", maxBulkServiceRawTargets))
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

	seenTargets := make(map[string]struct{}, len(req.Targets))
	filteredTargets := make([]string, 0, len(req.Targets))
	deniedTargets := make([]string, 0)
	for _, target := range req.Targets {
		normalizedTarget := strings.TrimSpace(target)
		if normalizedTarget == "" {
			apiv2.WriteError(w, http.StatusBadRequest, "validation", "targets must not contain empty asset ids")
			return
		}
		if _, exists := seenTargets[normalizedTarget]; exists {
			apiv2.WriteError(w, http.StatusBadRequest, "validation", "duplicate target: "+normalizedTarget)
			return
		}
		seenTargets[normalizedTarget] = struct{}{}
		if !apiv2.AssetCheck(allowed, normalizedTarget) {
			deniedTargets = append(deniedTargets, normalizedTarget)
			continue
		}
		if len(filteredTargets) >= maxBulkServiceTargets {
			apiv2.WriteError(w, http.StatusBadRequest, "validation",
				fmt.Sprintf("too many accessible targets: maximum is %d", maxBulkServiceTargets))
			return
		}
		filteredTargets = append(filteredTargets, normalizedTarget)
	}
	if len(deniedTargets) > 0 {
		apiv2.WriteError(w, http.StatusForbidden, "asset_forbidden", "api key does not have access to targets: "+strings.Join(deniedTargets, ", "))
		return
	}
	if len(filteredTargets) == 0 {
		apiv2.WriteError(w, http.StatusForbidden, "asset_forbidden", "none of the requested targets are accessible with this API key")
		return
	}
	for _, target := range filteredTargets {
		if !maintenanceguard.EnforceAssetAction(w, target, d.EvaluateAssetGuardrails) {
			return
		}
	}

	orderedResults := make([]ExecResult, len(filteredTargets))
	jobs := make(chan int)
	var wg sync.WaitGroup
	workerCount := min(maxBulkServiceConcurrency, len(filteredTargets))
	for worker := 0; worker < workerCount; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				orderedResults[index] = d.ExecOnAsset(r, filteredTargets[index], command, 30)
			}
		}()
	}
	for index := range filteredTargets {
		jobs <- index
	}
	close(jobs)
	wg.Wait()

	results := make(map[string]any, len(filteredTargets))
	for index, target := range filteredTargets {
		result := orderedResults[index]
		if result.Error != "" {
			results[target] = map[string]any{"error": result.Error, "message": result.Message}
		} else if result.ExitCode != 0 {
			results[target] = map[string]any{
				"status":    "failed",
				"exit_code": result.ExitCode,
				"output":    result.Stdout,
			}
		} else {
			results[target] = map[string]any{"status": "ok", "exit_code": 0, "output": result.Stdout}
		}
	}

	shared.AppendAuditEventBestEffort(d.AuditStore, audit.Event{
		Type:      "api.bulk.service_action",
		ActorID:   apiv2.PrincipalActorID(r.Context()),
		Details:   map[string]any{"action": req.Action, "service": req.Service, "targets": filteredTargets},
		Timestamp: time.Now().UTC(),
	}, "v2 bulk service action")

	apiv2.WriteJSON(w, http.StatusOK, map[string]any{"results": results})
}

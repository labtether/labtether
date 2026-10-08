package updatespkg

import (
	"errors"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/servicehttp"
	"github.com/labtether/labtether/internal/updates"
	"net/http"
	"strings"
)

// HandleUpdateRuns handles GET /updates/runs.
func (d *Deps) HandleUpdateRuns(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/updates/runs" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	runs, err := d.UpdateStore.ListUpdateRuns(shared.ParseLimit(r, 50), r.URL.Query().Get("status"))
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list update runs")
		return
	}

	if shared.HasAssetRestriction(r.Context()) {
		planAccess := make(map[string]bool, len(runs))
		filtered := make([]updates.Run, 0, len(runs))
		for _, run := range runs {
			allowed, cached := planAccess[run.PlanID]
			if !cached {
				plan, ok, loadErr := d.UpdateStore.GetUpdatePlan(run.PlanID)
				if loadErr != nil {
					servicehttp.WriteError(w, http.StatusInternalServerError, "failed to authorize update runs")
					return
				}
				allowed = ok && updatePlanAllowed(r.Context(), plan)
				planAccess[run.PlanID] = allowed
			}
			if allowed {
				filtered = append(filtered, run)
			}
		}
		runs = filtered
	}

	groupID := shared.GroupIDQueryParam(r)
	if groupID != "" {
		if d.GroupStore == nil {
			servicehttp.WriteError(w, http.StatusServiceUnavailable, "group store unavailable")
			return
		}
		if d.AssetStore == nil {
			servicehttp.WriteError(w, http.StatusServiceUnavailable, "asset store unavailable")
			return
		}
		_, ok, err := d.GroupStore.GetGroup(groupID)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load group")
			return
		}
		if !ok {
			servicehttp.WriteError(w, http.StatusNotFound, "group not found")
			return
		}
		if !d.requireGroupAccess(w, r, groupID) {
			return
		}

		assetList, err := d.AssetStore.ListAssets()
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list assets")
			return
		}
		assetSite := make(map[string]string, len(assetList))
		for _, assetEntry := range assetList {
			if strings.TrimSpace(assetEntry.GroupID) != "" {
				assetSite[assetEntry.ID] = strings.TrimSpace(assetEntry.GroupID)
			}
		}

		planGroupCache := make(map[string]bool, len(runs))
		filtered := make([]updates.Run, 0, len(runs))
		for _, run := range runs {
			touchesGroup, err := d.updateRunTouchesGroup(run.PlanID, groupID, assetSite, planGroupCache)
			if err != nil {
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to map update runs to group")
				return
			}
			if touchesGroup {
				filtered = append(filtered, run)
			}
		}
		runs = filtered
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"runs": runs})
}

// HandleUpdateRunActions handles GET and DELETE /updates/runs/{id}.
func (d *Deps) HandleUpdateRunActions(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/updates/runs/")
	if path == r.URL.Path || path == "" || strings.Contains(path, "/") {
		servicehttp.WriteError(w, http.StatusNotFound, "update run path not found")
		return
	}

	runID := strings.TrimSpace(path)
	switch r.Method {
	case http.MethodGet:
		run, ok, err := d.UpdateStore.GetUpdateRun(runID)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load update run")
			return
		}
		if !ok {
			servicehttp.WriteError(w, http.StatusNotFound, "update run not found")
			return
		}
		plan, planOK, err := d.UpdateStore.GetUpdatePlan(run.PlanID)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to authorize update run")
			return
		}
		if !planOK || !requireUpdatePlanAccess(w, r, plan) {
			if !planOK {
				servicehttp.WriteError(w, http.StatusForbidden, "update run plan is unavailable")
			}
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"run": run})
	case http.MethodDelete:
		run, ok, err := d.UpdateStore.GetUpdateRun(runID)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load update run")
			return
		}
		if !ok {
			servicehttp.WriteError(w, http.StatusNotFound, "update run not found")
			return
		}
		plan, planOK, err := d.UpdateStore.GetUpdatePlan(run.PlanID)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to authorize update run")
			return
		}
		if !planOK {
			servicehttp.WriteError(w, http.StatusForbidden, "update run plan is unavailable")
			return
		}
		if !requireUpdatePlanAccess(w, r, plan) {
			return
		}
		if err := d.UpdateStore.DeleteUpdateRun(runID); err != nil {
			if errors.Is(err, persistence.ErrNotFound) {
				servicehttp.WriteError(w, http.StatusNotFound, "update run not found")
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to delete update run")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"deleted": true, "run_id": runID})
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// updateRunTouchesGroup checks whether an update run's plan targets any asset
// in the given group. Results are cached in planGroupCache to avoid redundant
// store lookups within a single list request.
func (d *Deps) updateRunTouchesGroup(planID, groupID string, assetGroup map[string]string, planGroupCache map[string]bool) (bool, error) {
	if groupID == "" {
		return true, nil
	}
	if cached, ok := planGroupCache[planID]; ok {
		return cached, nil
	}
	if d.UpdateStore == nil {
		return false, nil
	}
	plan, ok, err := d.UpdateStore.GetUpdatePlan(planID)
	if err != nil {
		return false, err
	}
	if !ok {
		planGroupCache[planID] = false
		return false, nil
	}
	touches := shared.UpdatePlanTouchesGroup(plan, groupID, assetGroup)
	planGroupCache[planID] = touches
	return touches, nil
}

package updatespkg

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/jobqueue"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/servicehttp"
	"github.com/labtether/labtether/internal/updates"
)

const (
	maxPlanNameLength  = 120
	maxPlanTargetCount = 100
	maxPlanScopeCount  = 24
	maxTargetLength    = 255
	maxModeLength      = 32
	maxActorIDLength   = 64
)

// HandleUpdatePlans handles GET and POST /updates/plans.
func (d *Deps) HandleUpdatePlans(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/updates/plans" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}

	switch r.Method {
	case http.MethodGet:
		plans, err := d.UpdateStore.ListUpdatePlans(shared.ParseLimit(r, 50))
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list update plans")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"plans": plans})
	case http.MethodPost:
		if d.EnforceRateLimit != nil && !d.EnforceRateLimit(w, r, "updates.plan.create", 60, time.Minute) {
			return
		}
		var req updates.CreatePlanRequest
		if err := shared.DecodeJSONBody(w, r, &req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid update plan payload")
			return
		}
		req.Name = strings.TrimSpace(req.Name)
		if req.Name == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "name is required")
			return
		}
		if err := shared.ValidateMaxLen("name", req.Name, maxPlanNameLength); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		if len(req.Targets) > maxPlanTargetCount {
			servicehttp.WriteError(w, http.StatusBadRequest, "too many targets")
			return
		}
		if len(req.Scopes) > maxPlanScopeCount {
			servicehttp.WriteError(w, http.StatusBadRequest, "too many scopes")
			return
		}
		for i, target := range req.Targets {
			target = strings.TrimSpace(target)
			if err := shared.ValidateMaxLen("target", target, maxTargetLength); err != nil {
				servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
				return
			}
			req.Targets[i] = target
		}
		for i, scope := range req.Scopes {
			scope = strings.TrimSpace(scope)
			if err := shared.ValidateMaxLen("scope", scope, maxModeLength); err != nil {
				servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
				return
			}
			req.Scopes[i] = scope
		}
		plan, err := d.UpdateStore.CreateUpdatePlan(req)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to create update plan")
			return
		}
		servicehttp.WriteJSON(w, http.StatusCreated, map[string]any{"plan": plan})
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// HandleUpdatePlanActions handles GET, DELETE /updates/plans/{id} and
// POST /updates/plans/{id}/execute.
func (d *Deps) HandleUpdatePlanActions(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/updates/plans/")
	if path == r.URL.Path || path == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "update plan path not found")
		return
	}

	parts := strings.Split(path, "/")
	planID := strings.TrimSpace(parts[0])

	// GET/DELETE /updates/plans/{id}
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			plan, ok, err := d.UpdateStore.GetUpdatePlan(planID)
			if err != nil {
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load update plan")
				return
			}
			if !ok {
				servicehttp.WriteError(w, http.StatusNotFound, "update plan not found")
				return
			}
			servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"plan": plan})
		case http.MethodDelete:
			if err := d.UpdateStore.DeleteUpdatePlan(planID); err != nil {
				if errors.Is(err, persistence.ErrNotFound) {
					servicehttp.WriteError(w, http.StatusNotFound, "update plan not found")
					return
				}
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to delete update plan")
				return
			}
			servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"deleted": true, "plan_id": planID})
		default:
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	// POST /updates/plans/{id}/execute
	if len(parts) != 2 || parts[1] != "execute" {
		servicehttp.WriteError(w, http.StatusNotFound, "invalid update plan action path")
		return
	}
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if d.EnforceRateLimit != nil && !d.EnforceRateLimit(w, r, "updates.plan.execute", 120, time.Minute) {
		return
	}

	plan, ok, err := d.UpdateStore.GetUpdatePlan(planID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load update plan")
		return
	}
	if !ok {
		servicehttp.WriteError(w, http.StatusNotFound, "update plan not found")
		return
	}

	req := updates.ExecutePlanRequest{}
	if err := shared.DecodeJSONBody(w, r, &req); err != nil && err != io.EOF {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid execute payload")
		return
	}
	requestedActorID := strings.TrimSpace(req.ActorID)
	req.ActorID = apiv2.PrincipalActorID(r.Context())
	if err := shared.ValidateMaxLen("actor_id", req.ActorID, maxActorIDLength); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if d.ResolveGroupIDsForTargets != nil && d.EvaluateGuardrails != nil {
		groupIDs, err := d.ResolveGroupIDsForTargets(plan.Targets)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to resolve target groups")
			return
		}
		for groupID := range groupIDs {
			guardrails, err := d.EvaluateGuardrails(groupID, time.Now().UTC())
			if err != nil {
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to evaluate maintenance windows")
				return
			}
			if guardrails.BlockUpdates {
				servicehttp.WriteJSON(w, http.StatusLocked, map[string]any{
					"error":    "updates are blocked by active maintenance windows",
					"group_id": groupID,
					"windows":  guardrails.ActiveWindows,
				})
				return
			}
		}
	}

	run, err := d.UpdateStore.CreateUpdateRun(plan, req)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to create update run")
		return
	}

	job := updates.Job{
		JobID:       idgen.New("updjob"),
		RunID:       run.ID,
		ActorID:     run.ActorID,
		DryRun:      run.DryRun,
		Plan:        plan,
		RequestedAt: run.CreatedAt,
	}

	if d.JobQueue == nil {
		_ = d.UpdateStore.ApplyUpdateResult(updates.Result{
			JobID:       job.JobID,
			RunID:       run.ID,
			Status:      updates.StatusFailed,
			Summary:     "update queue unavailable",
			Error:       "update queue unavailable",
			CompletedAt: time.Now().UTC(),
		})
		auditDispatch := audit.NewEvent("updates.run.queued")
		auditDispatch.ActorID = run.ActorID
		auditDispatch.SessionID = run.ID
		auditDispatch.Decision = "failed"
		auditDispatch.Reason = "queue unavailable"
		auditDispatch.Details = map[string]any{
			"job_id":    job.JobID,
			"plan_id":   plan.ID,
			"dry_run":   run.DryRun,
			"transport": "postgres",
		}
		if d.AppendAuditEventBestEffort != nil {
			d.AppendAuditEventBestEffort(auditDispatch, "api warning: failed to append update queued audit event")
		}
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "update queue unavailable")
		return
	}

	payload, err := json.Marshal(job)
	if err != nil {
		_ = d.UpdateStore.ApplyUpdateResult(updates.Result{
			JobID:       job.JobID,
			RunID:       run.ID,
			Status:      updates.StatusFailed,
			Summary:     "failed to serialize update job",
			Error:       "marshal failed",
			CompletedAt: time.Now().UTC(),
		})
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to serialize update job")
		return
	}

	if _, err := d.JobQueue.Enqueue(r.Context(), jobqueue.KindUpdateRun, payload); err != nil {
		_ = d.UpdateStore.ApplyUpdateResult(updates.Result{
			JobID:       job.JobID,
			RunID:       run.ID,
			Status:      updates.StatusFailed,
			Summary:     "failed to enqueue update run",
			Error:       "enqueue failed",
			CompletedAt: time.Now().UTC(),
		})
		auditDispatch := audit.NewEvent("updates.run.queued")
		auditDispatch.ActorID = run.ActorID
		auditDispatch.SessionID = run.ID
		auditDispatch.Decision = "failed"
		auditDispatch.Reason = "enqueue failed"
		auditDispatch.Details = map[string]any{
			"job_id":    job.JobID,
			"plan_id":   plan.ID,
			"dry_run":   run.DryRun,
			"transport": "postgres",
		}
		if d.AppendAuditEventBestEffort != nil {
			d.AppendAuditEventBestEffort(auditDispatch, "api warning: failed to append update queued audit event")
		}
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to enqueue update run")
		return
	}

	auditQueued := audit.NewEvent("updates.run.queued")
	auditQueued.ActorID = run.ActorID
	auditQueued.SessionID = run.ID
	auditQueued.Decision = "queued"
	auditQueuedDetails := map[string]any{
		"job_id":    job.JobID,
		"plan_id":   plan.ID,
		"dry_run":   run.DryRun,
		"transport": "postgres",
	}
	if requestedActorID != "" && requestedActorID != req.ActorID {
		auditQueuedDetails["requested_actor_label"] = requestedActorID
	}
	auditQueued.Details = auditQueuedDetails
	if d.AppendAuditEventBestEffort != nil {
		d.AppendAuditEventBestEffort(auditQueued, "api warning: failed to append update queued audit event")
	}

	logFields := map[string]string{
		"run_id":  run.ID,
		"plan_id": plan.ID,
		"job_id":  job.JobID,
	}
	if requestedActorID != "" && requestedActorID != req.ActorID {
		logFields["requested_actor_label"] = requestedActorID
	}

	if d.LogStore != nil {
		if err := d.LogStore.AppendEvent(logs.Event{
			ID:        fmt.Sprintf("log_update_queued_%s", job.JobID),
			Source:    "updates",
			Level:     "info",
			Message:   fmt.Sprintf("update run queued: %s", plan.Name),
			Timestamp: run.CreatedAt,
			Fields:    logFields,
		}); err != nil {
			log.Printf("api warning: failed to append queued update log event: %v", err)
		}
	}

	response := map[string]any{
		"job_id": job.JobID,
		"run":    run,
		"queue":  "job_queue",
		"status": "queued",
	}
	servicehttp.WriteJSON(w, http.StatusAccepted, response)
}

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
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"run": run})
	case http.MethodDelete:
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

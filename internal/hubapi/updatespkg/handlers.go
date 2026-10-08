package updatespkg

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/jobqueue"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/servicehttp"
	"github.com/labtether/labtether/internal/updates"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
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
		if shared.HasAssetRestriction(r.Context()) {
			filtered := make([]updates.Plan, 0, len(plans))
			for _, plan := range plans {
				if updatePlanAllowed(r.Context(), plan) {
					filtered = append(filtered, plan)
				}
			}
			plans = filtered
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
		for _, target := range req.Targets {
			target = strings.TrimSpace(target)
			if err := shared.ValidateMaxLen("target", target, maxTargetLength); err != nil {
				servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
				return
			}
		}
		normalizedTargets, err := updates.NormalizeTargets(req.Targets)
		if err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		req.Targets = normalizedTargets
		if !shared.AllAssetsAllowed(r.Context(), req.Targets...) {
			apiv2.WriteError(w, http.StatusForbidden, "asset_forbidden", "api key may only create update plans containing explicitly allowed assets")
			return
		}
		for i, scope := range req.Scopes {
			scope = strings.TrimSpace(scope)
			if err := shared.ValidateMaxLen("scope", scope, maxModeLength); err != nil {
				servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
				return
			}
			req.Scopes[i] = scope
		}
		normalizedScopes, err := updates.NormalizeExecutableScopes(req.Scopes)
		if err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		req.Scopes = normalizedScopes
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
			if !requireUpdatePlanAccess(w, r, plan) {
				return
			}
			servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"plan": plan})
		case http.MethodDelete:
			plan, ok, err := d.UpdateStore.GetUpdatePlan(planID)
			if err != nil {
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load update plan")
				return
			}
			if !ok {
				servicehttp.WriteError(w, http.StatusNotFound, "update plan not found")
				return
			}
			if !requireUpdatePlanAccess(w, r, plan) {
				return
			}
			if err := d.UpdateStore.DeleteUpdatePlan(planID); err != nil {
				if errors.Is(err, persistence.ErrNotFound) {
					servicehttp.WriteError(w, http.StatusNotFound, "update plan not found")
					return
				}
				if errors.Is(err, persistence.ErrUpdatePlanActive) {
					apiv2.WriteError(
						w,
						http.StatusConflict,
						"update_plan_active",
						"Update plan cannot be deleted while runs are queued or running.",
					)
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
	if !requireUpdatePlanAccess(w, r, plan) {
		return
	}
	if _, err := updates.NormalizeExecutableScopes(plan.Scopes); err != nil {
		servicehttp.WriteError(w, http.StatusConflict, "update plan contains an unsupported scope; recreate the plan with a supported scope")
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
			ID:     fmt.Sprintf("log_update_queued_%s", job.JobID),
			Source: "updates",
			Level:  "info",
			// nosemgrep: go.lang.security.injection.tainted-sql-string.tainted-sql-string -- this formats a stored log message; it is never executed as SQL.
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

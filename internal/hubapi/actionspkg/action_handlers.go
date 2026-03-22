package actionspkg

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/jobqueue"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/policy"
	"github.com/labtether/labtether/internal/servicehttp"
)

const (
	maxActorIDLength        = 64
	maxTargetLength         = 255
	maxCommandLength        = 4096
	maxConnectorIDLength    = 64
	maxActionIDLength       = 64
	maxActionParamCount     = 24
	maxActionParamKeyLength = 64
	maxActionParamValLength = 512
)

// HandleActionExecute handles POST /actions/execute.
func (d *Deps) HandleActionExecute(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/actions/execute" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if d.EnforceRateLimit != nil && !d.EnforceRateLimit(w, r, "actions.execute", 120, time.Minute) {
		return
	}

	var req actions.ExecuteRequest
	if err := shared.DecodeJSONBody(w, r, &req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid action payload")
		return
	}

	requestedActorID := strings.TrimSpace(req.ActorID)
	req.ActorID = apiv2.PrincipalActorID(r.Context())
	if requestedActorID != "" && requestedActorID != req.ActorID {
		if req.Params == nil {
			req.Params = make(map[string]string, 1)
		}
		if len(req.Params) < maxActionParamCount {
			req.Params["requested_actor_label"] = requestedActorID
		}
	}
	req.Type = actions.NormalizeRunType(req.Type)
	if req.Type == "" {
		if strings.TrimSpace(req.ConnectorID) != "" || strings.TrimSpace(req.ActionID) != "" {
			req.Type = actions.RunTypeConnectorAction
		} else {
			req.Type = actions.RunTypeCommand
		}
	}
	req.Target = strings.TrimSpace(req.Target)
	req.Command = strings.TrimSpace(req.Command)
	req.ConnectorID = strings.TrimSpace(req.ConnectorID)
	req.ActionID = strings.TrimSpace(req.ActionID)
	if err := shared.ValidateMaxLen("actor_id", req.ActorID, maxActorIDLength); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := shared.ValidateMaxLen("target", req.Target, maxTargetLength); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := shared.ValidateMaxLen("command", req.Command, maxCommandLength); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := shared.ValidateMaxLen("connector_id", req.ConnectorID, maxConnectorIDLength); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := shared.ValidateMaxLen("action_id", req.ActionID, maxActionIDLength); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(req.Params) > maxActionParamCount {
		servicehttp.WriteError(w, http.StatusBadRequest, "too many action params")
		return
	}
	for key, value := range req.Params {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "action param key cannot be blank")
			return
		}
		if err := shared.ValidateMaxLen("action param key", key, maxActionParamKeyLength); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := shared.ValidateMaxLen("action param value", value, maxActionParamValLength); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		req.Params[key] = value
	}

	checkCommand := req.Command
	target := req.Target
	mode := "structured"
	actionName := "command_execute"

	switch req.Type {
	case actions.RunTypeCommand:
		if req.Target == "" || req.Command == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "command actions require target and command")
			return
		}
	case actions.RunTypeConnectorAction:
		if req.ConnectorID == "" || req.ActionID == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "connector actions require connector_id and action_id")
			return
		}
		if target == "" {
			target = req.ConnectorID
		}
		mode = "connector"
		actionName = "connector_action_execute"
		checkCommand = req.ConnectorID + ":" + req.ActionID
	default:
		servicehttp.WriteError(w, http.StatusBadRequest, "unsupported action type")
		return
	}

	var groupID string
	if d.ResolveGroupIDForAction != nil {
		var err error
		groupID, err = d.ResolveGroupIDForAction(req)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to resolve target group")
			return
		}
	}
	if d.EvaluateGuardrails != nil {
		guardrails, err := d.EvaluateGuardrails(groupID, time.Now().UTC())
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to evaluate maintenance windows")
			return
		}
		if guardrails.BlockActions {
			servicehttp.WriteJSON(w, http.StatusLocked, map[string]any{
				"error":    "actions are blocked by active maintenance windows",
				"group_id": groupID,
				"windows":  guardrails.ActiveWindows,
			})
			return
		}
	}

	var checkRes policy.CheckResponse
	if d.GetPolicyConfig != nil {
		checkRes = policy.Evaluate(policy.CheckRequest{
			ActorID: req.ActorID,
			Target:  target,
			Mode:    mode,
			Action:  actionName,
			Command: checkCommand,
		}, d.GetPolicyConfig())
	} else {
		checkRes = policy.CheckResponse{Allowed: true}
	}

	auditCheck := audit.NewEvent("actions.run.policy_checked")
	auditCheck.ActorID = req.ActorID
	auditCheck.Target = target
	auditCheck.Decision = "allowed"
	if !checkRes.Allowed {
		auditCheck.Decision = "denied"
		auditCheck.Reason = checkRes.Reason
	}
	auditCheckDetails := map[string]any{
		"type":         req.Type,
		"command":      req.Command,
		"connector_id": req.ConnectorID,
		"action_id":    req.ActionID,
	}
	if requestedActorID != "" && requestedActorID != req.ActorID {
		auditCheckDetails["requested_actor_label"] = requestedActorID
	}
	auditCheck.Details = auditCheckDetails
	if d.AppendAuditEventBestEffort != nil {
		d.AppendAuditEventBestEffort(auditCheck, "api warning: failed to append action policy audit event")
	}

	if !checkRes.Allowed {
		servicehttp.WriteError(w, http.StatusForbidden, checkRes.Reason)
		return
	}

	run, err := d.ActionStore.CreateActionRun(req)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to create action run")
		return
	}

	job := actions.Job{
		JobID:       idgen.New("actjob"),
		RunID:       run.ID,
		Type:        run.Type,
		ActorID:     run.ActorID,
		Target:      run.Target,
		Command:     run.Command,
		ConnectorID: run.ConnectorID,
		ActionID:    run.ActionID,
		Params:      run.Params,
		DryRun:      run.DryRun,
		RequestedAt: run.CreatedAt,
	}

	if d.JobQueue == nil {
		_ = d.ActionStore.ApplyActionResult(actions.Result{
			JobID:       job.JobID,
			RunID:       run.ID,
			Status:      actions.StatusFailed,
			Error:       "action queue unavailable",
			CompletedAt: time.Now().UTC(),
			Steps: []actions.StepResult{
				{Name: "dispatch", Status: actions.StatusFailed, Error: "queue unavailable"},
			},
		})
		auditDispatch := audit.NewEvent("actions.run.queued")
		auditDispatch.ActorID = run.ActorID
		auditDispatch.Target = run.Target
		auditDispatch.SessionID = run.ID
		auditDispatch.Decision = "failed"
		auditDispatch.Reason = "queue unavailable"
		auditDispatch.Details = map[string]any{
			"job_id":       job.JobID,
			"type":         run.Type,
			"connector_id": run.ConnectorID,
			"action_id":    run.ActionID,
			"transport":    "postgres",
		}
		if d.AppendAuditEventBestEffort != nil {
			d.AppendAuditEventBestEffort(auditDispatch, "api warning: failed to append action queued audit event")
		}
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "action queue unavailable")
		return
	}

	payload, err := json.Marshal(job)
	if err != nil {
		_ = d.ActionStore.ApplyActionResult(actions.Result{
			JobID:       job.JobID,
			RunID:       run.ID,
			Status:      actions.StatusFailed,
			Error:       "failed to serialize action job",
			CompletedAt: time.Now().UTC(),
			Steps: []actions.StepResult{
				{Name: "dispatch", Status: actions.StatusFailed, Error: "marshal failed"},
			},
		})
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to serialize action job")
		return
	}

	if _, err := d.JobQueue.Enqueue(r.Context(), jobqueue.KindActionRun, payload); err != nil {
		_ = d.ActionStore.ApplyActionResult(actions.Result{
			JobID:       job.JobID,
			RunID:       run.ID,
			Status:      actions.StatusFailed,
			Error:       "failed to enqueue action",
			CompletedAt: time.Now().UTC(),
			Steps: []actions.StepResult{
				{Name: "dispatch", Status: actions.StatusFailed, Error: "enqueue failed"},
			},
		})
		auditDispatch := audit.NewEvent("actions.run.queued")
		auditDispatch.ActorID = run.ActorID
		auditDispatch.Target = run.Target
		auditDispatch.SessionID = run.ID
		auditDispatch.Decision = "failed"
		auditDispatch.Reason = "enqueue failed"
		auditDispatch.Details = map[string]any{
			"job_id":       job.JobID,
			"type":         run.Type,
			"connector_id": run.ConnectorID,
			"action_id":    run.ActionID,
			"transport":    "postgres",
		}
		if d.AppendAuditEventBestEffort != nil {
			d.AppendAuditEventBestEffort(auditDispatch, "api warning: failed to append action queued audit event")
		}
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to enqueue action run")
		return
	}

	auditQueued := audit.NewEvent("actions.run.queued")
	auditQueued.ActorID = run.ActorID
	auditQueued.Target = run.Target
	auditQueued.SessionID = run.ID
	auditQueued.Decision = "queued"
	auditQueued.Details = map[string]any{
		"job_id":       job.JobID,
		"type":         run.Type,
		"connector_id": run.ConnectorID,
		"action_id":    run.ActionID,
		"transport":    "postgres",
	}
	if requestedActorID != "" && requestedActorID != req.ActorID {
		auditQueued.Details["requested_actor_label"] = requestedActorID
	}
	if d.AppendAuditEventBestEffort != nil {
		d.AppendAuditEventBestEffort(auditQueued, "api warning: failed to append action queued audit event")
	}

	logFields := map[string]string{
		"run_id":       run.ID,
		"job_id":       job.JobID,
		"type":         run.Type,
		"connector_id": run.ConnectorID,
		"action_id":    run.ActionID,
	}
	if d.LogStore != nil {
		if err := d.LogStore.AppendEvent(logs.Event{
			ID:        fmt.Sprintf("log_action_queued_%s", job.JobID),
			AssetID:   run.Target,
			Source:    "actions",
			Level:     "info",
			Message:   fmt.Sprintf("action queued: %s", run.Type),
			Timestamp: run.CreatedAt,
			Fields:    logFields,
		}); err != nil {
			log.Printf("api warning: failed to append queued action log event: %v", err)
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

// HandleActionRuns handles GET /actions/runs.
func (d *Deps) HandleActionRuns(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/actions/runs" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	runs, err := d.ActionStore.ListActionRuns(
		shared.ParseLimit(r, 50),
		shared.ParseOffset(r),
		r.URL.Query().Get("type"),
		r.URL.Query().Get("status"),
	)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list action runs")
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

		filtered := make([]actions.Run, 0, len(runs))
		for _, run := range runs {
			if shared.ActionRunMatchesGroup(run, groupID, assetSite) {
				filtered = append(filtered, run)
			}
		}
		runs = filtered
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"runs": runs})
}

// HandleActionRunActions handles GET and DELETE /actions/runs/{id}.
func (d *Deps) HandleActionRunActions(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/actions/runs/")
	if path == r.URL.Path || path == "" || strings.Contains(path, "/") {
		servicehttp.WriteError(w, http.StatusNotFound, "action run path not found")
		return
	}

	runID := strings.TrimSpace(path)
	switch r.Method {
	case http.MethodGet:
		run, ok, err := d.ActionStore.GetActionRun(runID)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load action run")
			return
		}
		if !ok {
			servicehttp.WriteError(w, http.StatusNotFound, "action run not found")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"run": run})
	case http.MethodDelete:
		if err := d.ActionStore.DeleteActionRun(runID); err != nil {
			if errors.Is(err, persistence.ErrNotFound) {
				servicehttp.WriteError(w, http.StatusNotFound, "action run not found")
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to delete action run")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"deleted": true, "run_id": runID})
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

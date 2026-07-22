package actionspkg

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/policy"
	"github.com/labtether/labtether/internal/savedactions"
)

const (
	maxSavedActionNameLength        = 200
	maxSavedActionDescriptionLength = 1000
	maxSavedActionStepCount         = 50
	maxSavedActionStepNameLength    = 200
	maxSavedActionPageSize          = 100
	maxSavedActionRunDuration       = 2 * time.Minute
	// defaultExecTimeout mirrors the constant of the same name in cmd/labtether.
	defaultExecTimeout = 30
)

// HandleV2SavedActions routes collection-level saved-action requests (GET list, POST create).
func (d *Deps) HandleV2SavedActions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if !apiv2.ScopeCheck(apiv2.ScopesFromContext(r.Context()), "actions:read") {
			apiv2.WriteScopeForbidden(w, "actions:read")
			return
		}
		d.V2ListSavedActions(w, r)
	case http.MethodPost:
		if !apiv2.ScopeCheck(apiv2.ScopesFromContext(r.Context()), "actions:write") {
			apiv2.WriteScopeForbidden(w, "actions:write")
			return
		}
		d.V2CreateSavedAction(w, r)
	default:
		apiv2.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	}
}

// HandleV2SavedActionActions routes per-resource requests for /api/v2/actions/{id}
// (GET, DELETE) and /api/v2/actions/{id}/run (POST).
func (d *Deps) HandleV2SavedActionActions(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v2/actions/")
	if path == "" || path == r.URL.Path {
		writeSavedActionNotFound(w)
		return
	}

	// Handle /api/v2/actions/{id}/run
	parts := strings.Split(path, "/")
	if len(parts) == 2 && parts[1] == "run" {
		id := parts[0]
		if !validSavedActionID(id) {
			writeSavedActionNotFound(w)
			return
		}
		if r.Method != http.MethodPost {
			apiv2.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST required")
			return
		}
		if !apiv2.ScopeCheck(apiv2.ScopesFromContext(r.Context()), "actions:exec") {
			apiv2.WriteScopeForbidden(w, "actions:exec")
			return
		}
		d.V2RunSavedAction(w, r, id)
		return
	}

	if len(parts) != 1 || !validSavedActionID(parts[0]) {
		writeSavedActionNotFound(w)
		return
	}
	id := parts[0]
	switch r.Method {
	case http.MethodGet:
		if !apiv2.ScopeCheck(apiv2.ScopesFromContext(r.Context()), "actions:read") {
			apiv2.WriteScopeForbidden(w, "actions:read")
			return
		}
		d.V2GetSavedAction(w, r, id)
	case http.MethodDelete:
		if !apiv2.ScopeCheck(apiv2.ScopesFromContext(r.Context()), "actions:write") {
			apiv2.WriteScopeForbidden(w, "actions:write")
			return
		}
		d.V2DeleteSavedAction(w, r, id)
	default:
		apiv2.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	}
}

// V2ListSavedActions returns all saved actions.
func (d *Deps) V2ListSavedActions(w http.ResponseWriter, r *http.Request) {
	if d.SavedActionStore == nil || d.AssetStore == nil {
		apiv2.WriteError(w, http.StatusServiceUnavailable, "unavailable", "saved action service is unavailable")
		return
	}
	limit := shared.ParseLimit(r, maxSavedActionPageSize)
	if limit > maxSavedActionPageSize {
		limit = maxSavedActionPageSize
	}
	offset := shared.ParseOffset(r)
	list, _, err := d.SavedActionStore.ListSavedActions(r.Context(), apiv2.PrincipalActorID(r.Context()), savedactions.MaxActionsPerActor, 0)
	if err != nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to list saved actions")
		return
	}
	assetsByID, err := d.savedActionAssetIndex()
	if err != nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to authorize saved actions")
		return
	}
	visible := make([]savedactions.SavedAction, 0, len(list))
	for _, action := range list {
		if savedActionAccessible(r.Context(), action, assetsByID) {
			visible = append(visible, action)
		}
	}
	total := len(visible)
	if offset >= total {
		visible = []savedactions.SavedAction{}
	} else {
		end := offset + limit
		if end > total {
			end = total
		}
		visible = visible[offset:end]
	}
	page := 1
	if limit > 0 {
		page = (offset / limit) + 1
	}
	apiv2.WriteList(w, http.StatusOK, visible, total, page, limit)
}

// V2CreateSavedAction creates a new saved action.
func (d *Deps) V2CreateSavedAction(w http.ResponseWriter, r *http.Request) {
	if d.SavedActionStore == nil || d.AssetStore == nil {
		apiv2.WriteError(w, http.StatusServiceUnavailable, "unavailable", "saved action service is unavailable")
		return
	}

	var req savedactions.CreateRequest
	if err := shared.DecodeJSONBody(w, r, &req); err != nil {
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "name is required")
		return
	}
	if len(req.Name) > maxSavedActionNameLength {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "name exceeds maximum length")
		return
	}
	req.Description = strings.TrimSpace(req.Description)
	if len(req.Description) > maxSavedActionDescriptionLength {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "description exceeds maximum length")
		return
	}
	if len(req.Steps) == 0 {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "at least one step is required")
		return
	}
	if len(req.Steps) > maxSavedActionStepCount {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "too many steps")
		return
	}

	normalizedSteps := make([]savedactions.ActionStep, 0, len(req.Steps))
	for i, step := range req.Steps {
		step.Name = strings.TrimSpace(step.Name)
		step.Command = strings.TrimSpace(step.Command)
		step.Target = strings.TrimSpace(step.Target)
		if step.Name == "" {
			step.Name = "step " + strconv.Itoa(i+1)
		}
		if step.Command == "" {
			apiv2.WriteError(w, http.StatusBadRequest, "validation", "step command is required")
			return
		}
		if step.Target == "" {
			apiv2.WriteError(w, http.StatusBadRequest, "validation", "step target is required")
			return
		}
		if len(step.Name) > maxSavedActionStepNameLength {
			apiv2.WriteError(w, http.StatusBadRequest, "validation", "step name exceeds maximum length")
			return
		}
		if len(step.Command) > maxCommandLength {
			apiv2.WriteError(w, http.StatusBadRequest, "validation", "step command exceeds maximum length")
			return
		}
		if len(step.Target) > maxTargetLength {
			apiv2.WriteError(w, http.StatusBadRequest, "validation", "step target exceeds maximum length")
			return
		}
		normalizedSteps = append(normalizedSteps, step)
	}
	assetsByID, err := d.savedActionAssetIndex()
	if err != nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to validate saved action targets")
		return
	}
	for _, step := range normalizedSteps {
		if !apiv2.AssetCheckContext(r.Context(), step.Target) {
			apiv2.WriteError(w, http.StatusForbidden, "asset_forbidden", "one or more step targets are not accessible")
			return
		}
		if _, ok := assetsByID[step.Target]; !ok {
			apiv2.WriteError(w, http.StatusBadRequest, "validation", "one or more step targets do not reference an existing asset")
			return
		}
	}

	now := time.Now().UTC()
	action := savedactions.SavedAction{
		ID:          idgen.New("act"),
		Name:        req.Name,
		Description: req.Description,
		Steps:       normalizedSteps,
		CreatedBy:   apiv2.PrincipalActorID(r.Context()),
		CreatedAt:   now,
	}

	if err := d.SavedActionStore.CreateSavedAction(r.Context(), action); err != nil {
		if errors.Is(err, savedactions.ErrCapacity) {
			apiv2.WriteError(w, http.StatusConflict, "capacity_reached", "saved action capacity reached")
			return
		}
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to create saved action")
		return
	}
	d.auditSavedAction(r, "saved_action.created", action, "allowed", "", map[string]any{
		"name_bytes":        len([]byte(action.Name)),
		"description_bytes": len([]byte(action.Description)),
	})

	apiv2.WriteJSON(w, http.StatusCreated, action)
}

// V2GetSavedAction returns a single saved action by ID.
func (d *Deps) V2GetSavedAction(w http.ResponseWriter, r *http.Request, id string) {
	if d.SavedActionStore == nil || d.AssetStore == nil {
		apiv2.WriteError(w, http.StatusServiceUnavailable, "unavailable", "saved action service is unavailable")
		return
	}
	action, ok, err := d.getAccessibleSavedAction(r, id)
	if err != nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to authorize saved action")
		return
	}
	if !ok {
		writeSavedActionNotFound(w)
		return
	}
	apiv2.WriteJSON(w, http.StatusOK, action)
}

// V2DeleteSavedAction removes a saved action by ID.
func (d *Deps) V2DeleteSavedAction(w http.ResponseWriter, r *http.Request, id string) {
	if d.SavedActionStore == nil || d.AssetStore == nil {
		apiv2.WriteError(w, http.StatusServiceUnavailable, "unavailable", "saved action service is unavailable")
		return
	}
	action, ok, err := d.getAccessibleSavedAction(r, id)
	if err != nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to authorize saved action")
		return
	}
	if !ok {
		writeSavedActionNotFound(w)
		return
	}
	if err := d.SavedActionStore.DeleteSavedAction(r.Context(), apiv2.PrincipalActorID(r.Context()), id); err != nil {
		if err == persistence.ErrNotFound {
			writeSavedActionNotFound(w)
			return
		}
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to delete saved action")
		return
	}
	d.auditSavedAction(r, "saved_action.deleted", action, "allowed", "", nil)
	apiv2.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// V2RunSavedAction loads the saved action and executes each step sequentially.
func (d *Deps) V2RunSavedAction(w http.ResponseWriter, r *http.Request, id string) {
	if !apiv2.ScopeCheck(apiv2.ScopesFromContext(r.Context()), "assets:exec") {
		apiv2.WriteScopeForbidden(w, "assets:exec")
		return
	}
	if d.EnforceRateLimit != nil && !d.EnforceRateLimit(w, r, "actions.execute", 120, time.Minute) {
		return
	}
	if d.SavedActionStore == nil || d.AssetStore == nil || d.ExecOnAsset == nil {
		apiv2.WriteError(w, http.StatusServiceUnavailable, "unavailable", "saved action execution is unavailable")
		return
	}

	action, ok, err := d.getAccessibleSavedAction(r, id)
	if err != nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to authorize saved action")
		return
	}
	if !ok {
		writeSavedActionNotFound(w)
		return
	}
	if len(action.Steps) == 0 || len(action.Steps) > maxSavedActionStepCount {
		apiv2.WriteError(w, http.StatusConflict, "invalid_saved_action", "saved action has an invalid step count")
		return
	}

	startedAt := time.Now()
	stepResults := make([]map[string]any, 0, len(action.Steps))
	preflight := make([]map[string]any, len(action.Steps))
	for index, step := range action.Steps {
		if deniedEntry, denied := d.savedActionDeniedStep(r, action, index, step); denied {
			preflight[index] = deniedEntry
		}
	}

	failedSteps := 0
	for index, step := range action.Steps {
		if preflight[index] != nil {
			failedSteps++
			stepResults = append(stepResults, preflight[index])
			continue
		}
		remaining := maxSavedActionRunDuration - time.Since(startedAt)
		if remaining <= 0 {
			failedSteps++
			stepResults = append(stepResults, map[string]any{
				"name":    step.Name,
				"target":  step.Target,
				"error":   "run_deadline_exceeded",
				"message": "saved action run reached its time limit",
			})
			continue
		}
		timeoutSec := defaultExecTimeout
		if remaining < time.Duration(timeoutSec)*time.Second {
			timeoutSec = max(1, int((remaining+time.Second-1)/time.Second))
		}
		result := d.ExecOnAsset(r, step.Target, step.Command, timeoutSec)
		entry := map[string]any{
			"name":   step.Name,
			"target": step.Target,
		}
		if result.Error != "" {
			failedSteps++
			entry["error"] = result.Error
			entry["message"] = result.Message
		} else {
			if result.ExitCode != 0 {
				failedSteps++
			}
			entry["exit_code"] = result.ExitCode
			entry["output"] = result.Stdout
			entry["duration_ms"] = result.DurationMs
		}
		stepResults = append(stepResults, entry)
	}
	d.auditSavedAction(r, "saved_action.run.completed", action, "allowed", "", map[string]any{
		"failed_steps": failedSteps,
		"duration_ms":  time.Since(startedAt).Milliseconds(),
	})

	apiv2.WriteJSON(w, http.StatusOK, map[string]any{
		"action_id": id,
		"steps":     stepResults,
	})
}

func (d *Deps) savedActionDeniedStep(r *http.Request, action savedactions.SavedAction, stepIndex int, step savedactions.ActionStep) (map[string]any, bool) {
	req := actions.ExecuteRequest{
		Type:    actions.RunTypeCommand,
		ActorID: apiv2.PrincipalActorID(r.Context()),
		Target:  step.Target,
		Command: step.Command,
	}

	var groupID string
	if d.ResolveGroupIDForAction != nil {
		resolvedGroupID, err := d.ResolveGroupIDForAction(req)
		if err != nil {
			return map[string]any{
				"name":    step.Name,
				"target":  step.Target,
				"error":   "group_resolution_failed",
				"message": "failed to resolve target group",
			}, true
		}
		groupID = resolvedGroupID
	}

	if d.EvaluateGuardrails != nil {
		guardrails, err := d.EvaluateGuardrails(groupID, time.Now().UTC())
		if err != nil {
			return map[string]any{
				"name":    step.Name,
				"target":  step.Target,
				"error":   "guardrail_evaluation_failed",
				"message": "failed to evaluate maintenance windows",
			}, true
		}
		if guardrails.BlockActions {
			return map[string]any{
				"name":     step.Name,
				"target":   step.Target,
				"error":    "maintenance_blocked",
				"message":  "actions are blocked by active maintenance windows",
				"group_id": groupID,
				"windows":  guardrails.ActiveWindows,
			}, true
		}
	}

	checkRes := policy.CheckResponse{Allowed: true}
	if d.GetPolicyConfig != nil {
		checkRes = policy.Evaluate(policy.CheckRequest{
			ActorID: req.ActorID,
			Target:  req.Target,
			Mode:    "structured",
			Action:  "command_execute",
			Command: req.Command,
		}, d.GetPolicyConfig())
	}
	if d.AppendAuditEventBestEffort != nil {
		event := audit.NewEvent("actions.run.policy_checked")
		event.ActorID = req.ActorID
		event.Target = req.Target
		event.Decision = "allowed"
		if !checkRes.Allowed {
			event.Decision = "denied"
			event.Reason = checkRes.Reason
		}
		event.Details = map[string]any{
			"type":             req.Type,
			"command_bytes":    len([]byte(req.Command)),
			"saved_action_id":  action.ID,
			"saved_step_index": stepIndex,
		}
		d.AppendAuditEventBestEffort(event, "api warning: failed to append saved action policy audit event")
	}
	if !checkRes.Allowed {
		return map[string]any{
			"name":    step.Name,
			"target":  step.Target,
			"error":   "policy_denied",
			"message": checkRes.Reason,
		}, true
	}
	return nil, false
}

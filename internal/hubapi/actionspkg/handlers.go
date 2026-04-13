package actionspkg

import (
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
		apiv2.WriteError(w, http.StatusNotFound, "not_found", "action id required")
		return
	}

	// Handle /api/v2/actions/{id}/run
	if strings.HasSuffix(path, "/run") {
		id := strings.TrimSuffix(path, "/run")
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

	id := path
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
	if d.SavedActionStore == nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "saved action store not configured")
		return
	}
	limit := shared.ParseLimit(r, 100)
	offset := shared.ParseOffset(r)
	list, total, err := d.SavedActionStore.ListSavedActions(r.Context(), apiv2.PrincipalActorID(r.Context()), limit, offset)
	if err != nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to list saved actions")
		return
	}
	if list == nil {
		list = []savedactions.SavedAction{}
	}
	page := 1
	if limit > 0 {
		page = (offset / limit) + 1
	}
	apiv2.WriteList(w, http.StatusOK, list, total, page, limit)
}

// V2CreateSavedAction creates a new saved action.
func (d *Deps) V2CreateSavedAction(w http.ResponseWriter, r *http.Request) {
	if d.SavedActionStore == nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "saved action store not configured")
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
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to create saved action")
		return
	}

	apiv2.WriteJSON(w, http.StatusCreated, action)
}

// V2GetSavedAction returns a single saved action by ID.
func (d *Deps) V2GetSavedAction(w http.ResponseWriter, r *http.Request, id string) {
	if d.SavedActionStore == nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "saved action store not configured")
		return
	}
	action, ok, err := d.SavedActionStore.GetSavedAction(r.Context(), apiv2.PrincipalActorID(r.Context()), id)
	if err != nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to get saved action")
		return
	}
	if !ok {
		apiv2.WriteError(w, http.StatusNotFound, "not_found", "no saved action with id: "+id)
		return
	}
	apiv2.WriteJSON(w, http.StatusOK, action)
}

// V2DeleteSavedAction removes a saved action by ID.
func (d *Deps) V2DeleteSavedAction(w http.ResponseWriter, r *http.Request, id string) {
	if d.SavedActionStore == nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "saved action store not configured")
		return
	}
	if err := d.SavedActionStore.DeleteSavedAction(r.Context(), apiv2.PrincipalActorID(r.Context()), id); err != nil {
		if err == persistence.ErrNotFound {
			apiv2.WriteError(w, http.StatusNotFound, "not_found", "no saved action with id: "+id)
			return
		}
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to delete saved action")
		return
	}
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
	if d.SavedActionStore == nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "saved action store not configured")
		return
	}

	action, ok, err := d.SavedActionStore.GetSavedAction(r.Context(), apiv2.PrincipalActorID(r.Context()), id)
	if err != nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to get saved action")
		return
	}
	if !ok {
		apiv2.WriteError(w, http.StatusNotFound, "not_found", "no saved action with id: "+id)
		return
	}

	allowed := apiv2.AllowedAssetsFromContext(r.Context())
	stepResults := make([]map[string]any, 0, len(action.Steps))

	for _, step := range action.Steps {
		if !apiv2.AssetCheck(allowed, step.Target) {
			stepResults = append(stepResults, map[string]any{
				"name":   step.Name,
				"target": step.Target,
				"error":  "asset_not_allowed",
			})
			continue
		}
		if deniedEntry, ok := d.savedActionDeniedStep(r, action, step); ok {
			stepResults = append(stepResults, deniedEntry)
			continue
		}
		result := d.ExecOnAsset(r, step.Target, step.Command, defaultExecTimeout)
		entry := map[string]any{
			"name":   step.Name,
			"target": step.Target,
		}
		if result.Error != "" {
			entry["error"] = result.Error
			entry["message"] = result.Message
		} else {
			entry["exit_code"] = result.ExitCode
			entry["output"] = result.Stdout
			entry["duration_ms"] = result.DurationMs
		}
		stepResults = append(stepResults, entry)
	}

	apiv2.WriteJSON(w, http.StatusOK, map[string]any{
		"action_id": id,
		"steps":     stepResults,
	})
}

func (d *Deps) savedActionDeniedStep(r *http.Request, action savedactions.SavedAction, step savedactions.ActionStep) (map[string]any, bool) {
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
				"command": step.Command,
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
				"command": step.Command,
				"error":   "guardrail_evaluation_failed",
				"message": "failed to evaluate maintenance windows",
			}, true
		}
		if guardrails.BlockActions {
			return map[string]any{
				"name":     step.Name,
				"target":   step.Target,
				"command":  step.Command,
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
			"type":            req.Type,
			"command":         req.Command,
			"saved_action_id": action.ID,
			"saved_step_name": step.Name,
		}
		d.AppendAuditEventBestEffort(event, "api warning: failed to append saved action policy audit event")
	}
	if !checkRes.Allowed {
		return map[string]any{
			"name":    step.Name,
			"target":  step.Target,
			"command": step.Command,
			"error":   "policy_denied",
			"message": checkRes.Reason,
		}, true
	}
	return nil, false
}

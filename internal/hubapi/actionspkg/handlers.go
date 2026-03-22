package actionspkg

import (
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/savedactions"
)

const (
	maxSavedActionNameLength = 200
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
	list, err := d.SavedActionStore.ListSavedActions(r.Context())
	if err != nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to list saved actions")
		return
	}
	if list == nil {
		list = []savedactions.SavedAction{}
	}
	apiv2.WriteList(w, http.StatusOK, list, len(list), 1, len(list))
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
	if len(req.Steps) == 0 {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "at least one step is required")
		return
	}

	now := time.Now().UTC()
	action := savedactions.SavedAction{
		ID:          idgen.New("act"),
		Name:        req.Name,
		Description: strings.TrimSpace(req.Description),
		Steps:       req.Steps,
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
	action, ok, err := d.SavedActionStore.GetSavedAction(r.Context(), id)
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
	if err := d.SavedActionStore.DeleteSavedAction(r.Context(), id); err != nil {
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
	if d.SavedActionStore == nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "saved action store not configured")
		return
	}

	action, ok, err := d.SavedActionStore.GetSavedAction(r.Context(), id)
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

package schedulespkg

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/schedules"
)

const (
	maxScheduleNameLength   = 200
	maxScheduleTargetsCount = 500
)

// HandleV2Schedules routes collection-level schedule requests (GET list, POST create).
func (d *Deps) HandleV2Schedules(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if !apiv2.ScopeCheck(apiv2.ScopesFromContext(r.Context()), "schedules:read") {
			apiv2.WriteScopeForbidden(w, "schedules:read")
			return
		}
		d.V2ListSchedules(w, r)
	case http.MethodPost:
		if !apiv2.ScopeCheck(apiv2.ScopesFromContext(r.Context()), "schedules:write") {
			apiv2.WriteScopeForbidden(w, "schedules:write")
			return
		}
		d.V2CreateSchedule(w, r)
	default:
		apiv2.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	}
}

// HandleV2ScheduleActions routes per-resource schedule requests (GET, PATCH, DELETE).
func (d *Deps) HandleV2ScheduleActions(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v2/schedules/")
	if id == "" || id == r.URL.Path || strings.Contains(id, "/") {
		apiv2.WriteError(w, http.StatusNotFound, "not_found", "schedule id required")
		return
	}

	switch r.Method {
	case http.MethodGet:
		if !apiv2.ScopeCheck(apiv2.ScopesFromContext(r.Context()), "schedules:read") {
			apiv2.WriteScopeForbidden(w, "schedules:read")
			return
		}
		d.V2GetSchedule(w, r, id)
	case http.MethodPatch, http.MethodPut:
		if !apiv2.ScopeCheck(apiv2.ScopesFromContext(r.Context()), "schedules:write") {
			apiv2.WriteScopeForbidden(w, "schedules:write")
			return
		}
		d.V2UpdateSchedule(w, r, id)
	case http.MethodDelete:
		if !apiv2.ScopeCheck(apiv2.ScopesFromContext(r.Context()), "schedules:write") {
			apiv2.WriteScopeForbidden(w, "schedules:write")
			return
		}
		d.V2DeleteSchedule(w, r, id)
	default:
		apiv2.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	}
}

// V2ListSchedules returns all scheduled tasks.
func (d *Deps) V2ListSchedules(w http.ResponseWriter, r *http.Request) {
	if d.ScheduleStore == nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "schedule store not configured")
		return
	}
	tasks, err := d.ScheduleStore.ListScheduledTasks(r.Context())
	if err != nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to list schedules")
		return
	}
	if tasks == nil {
		tasks = []schedules.ScheduledTask{}
	}
	apiv2.WriteList(w, http.StatusOK, tasks, len(tasks), 1, len(tasks))
}

// V2CreateSchedule creates a new scheduled task.
func (d *Deps) V2CreateSchedule(w http.ResponseWriter, r *http.Request) {
	if d.ScheduleStore == nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "schedule store not configured")
		return
	}

	var req schedules.CreateRequest
	if err := shared.DecodeJSONBody(w, r, &req); err != nil {
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "name is required")
		return
	}
	if len(req.Name) > maxScheduleNameLength {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "name exceeds maximum length")
		return
	}
	req.CronExpr = strings.TrimSpace(req.CronExpr)
	if req.CronExpr == "" {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "cron_expr is required")
		return
	}
	req.Command = strings.TrimSpace(req.Command)
	if req.Command == "" {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "command is required")
		return
	}
	req.GroupID = strings.TrimSpace(req.GroupID)
	req.Targets = normalizeScheduleTargets(req.Targets)
	if len(req.Targets) > maxScheduleTargetsCount {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "too many targets")
		return
	}

	now := time.Now().UTC()
	task := schedules.ScheduledTask{
		ID:        idgen.New("sched"),
		Name:      req.Name,
		CronExpr:  req.CronExpr,
		Command:   req.Command,
		Targets:   req.Targets,
		GroupID:   req.GroupID,
		Enabled:   true,
		CreatedBy: apiv2.PrincipalActorID(r.Context()),
		CreatedAt: now,
	}
	if task.Targets == nil {
		task.Targets = []string{}
	}

	if err := d.ScheduleStore.CreateScheduledTask(r.Context(), task); err != nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to create schedule")
		return
	}

	apiv2.WriteJSON(w, http.StatusCreated, task)
}

// V2GetSchedule returns a single scheduled task by ID.
func (d *Deps) V2GetSchedule(w http.ResponseWriter, r *http.Request, id string) {
	if d.ScheduleStore == nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "schedule store not configured")
		return
	}
	task, ok, err := d.ScheduleStore.GetScheduledTask(r.Context(), id)
	if err != nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to get schedule")
		return
	}
	if !ok {
		apiv2.WriteError(w, http.StatusNotFound, "not_found", "no schedule with id: "+id)
		return
	}
	apiv2.WriteJSON(w, http.StatusOK, task)
}

// V2UpdateSchedule applies a partial update to an existing scheduled task.
func (d *Deps) V2UpdateSchedule(w http.ResponseWriter, r *http.Request, id string) {
	if d.ScheduleStore == nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "schedule store not configured")
		return
	}

	var req struct {
		Name     *string   `json:"name,omitempty"`
		CronExpr *string   `json:"cron_expr,omitempty"`
		Command  *string   `json:"command,omitempty"`
		Targets  *[]string `json:"targets,omitempty"`
		GroupID  *string   `json:"group_id,omitempty"`
		Enabled  *bool     `json:"enabled,omitempty"`
	}
	if err := shared.DecodeJSONBody(w, r, &req); err != nil {
		return
	}

	if req.Name == nil && req.CronExpr == nil && req.Command == nil && req.Targets == nil && req.GroupID == nil && req.Enabled == nil {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "at least one field required")
		return
	}

	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		if trimmed == "" {
			apiv2.WriteError(w, http.StatusBadRequest, "validation", "name cannot be empty")
			return
		}
		if len(trimmed) > maxScheduleNameLength {
			apiv2.WriteError(w, http.StatusBadRequest, "validation", "name exceeds maximum length")
			return
		}
		req.Name = &trimmed
	}
	if req.CronExpr != nil {
		trimmed := strings.TrimSpace(*req.CronExpr)
		if trimmed == "" {
			apiv2.WriteError(w, http.StatusBadRequest, "validation", "cron_expr cannot be empty")
			return
		}
		req.CronExpr = &trimmed
	}
	if req.Command != nil {
		trimmed := strings.TrimSpace(*req.Command)
		if trimmed == "" {
			apiv2.WriteError(w, http.StatusBadRequest, "validation", "command cannot be empty")
			return
		}
		req.Command = &trimmed
	}
	if req.Targets != nil {
		normalizedTargets := normalizeScheduleTargets(*req.Targets)
		if len(normalizedTargets) > maxScheduleTargetsCount {
			apiv2.WriteError(w, http.StatusBadRequest, "validation", "too many targets")
			return
		}
		req.Targets = &normalizedTargets
	}
	if req.GroupID != nil {
		trimmed := strings.TrimSpace(*req.GroupID)
		req.GroupID = &trimmed
	}

	if err := d.ScheduleStore.UpdateScheduledTask(r.Context(), id, req.Name, req.CronExpr, req.Command, req.Targets, req.GroupID, req.Enabled); err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			apiv2.WriteError(w, http.StatusNotFound, "not_found", "schedule not found")
			return
		}
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to update schedule")
		return
	}

	task, ok, err := d.ScheduleStore.GetScheduledTask(r.Context(), id)
	if err != nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to retrieve updated schedule")
		return
	}
	if !ok {
		apiv2.WriteError(w, http.StatusNotFound, "not_found", "schedule not found")
		return
	}
	apiv2.WriteJSON(w, http.StatusOK, task)
}

// V2DeleteSchedule removes a scheduled task by ID.
func (d *Deps) V2DeleteSchedule(w http.ResponseWriter, r *http.Request, id string) {
	if d.ScheduleStore == nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "schedule store not configured")
		return
	}
	if err := d.ScheduleStore.DeleteScheduledTask(r.Context(), id); err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			apiv2.WriteError(w, http.StatusNotFound, "not_found", "schedule not found")
			return
		}
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to delete schedule")
		return
	}
	apiv2.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func normalizeScheduleTargets(targets []string) []string {
	if len(targets) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(targets))
	seen := make(map[string]struct{}, len(targets))
	for _, target := range targets {
		trimmed := strings.TrimSpace(target)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

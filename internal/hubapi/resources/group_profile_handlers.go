package resources

import (
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/groupprofiles"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/servicehttp"
)

func (d *Deps) HandleGroupProfiles(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/group-profiles" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if d.GroupProfileStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "group profile store unavailable")
		return
	}

	switch r.Method {
	case http.MethodGet:
		profiles, err := d.GroupProfileStore.ListGroupProfiles(parseLimit(r, 50))
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list group profiles")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"profiles": profiles})
	case http.MethodPost:
		if !d.EnforceRateLimit(w, r, "groupprofiles.create", 120, time.Minute) {
			return
		}
		var req groupprofiles.CreateProfileRequest
		if err := d.DecodeJSONBody(w, r, &req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid group profile payload")
			return
		}
		req.Name = strings.TrimSpace(req.Name)
		req.Description = strings.TrimSpace(req.Description)
		if req.Name == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "name is required")
			return
		}
		if req.Config == nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "config is required")
			return
		}
		profile, err := d.GroupProfileStore.CreateGroupProfile(req)
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "duplicate") ||
				strings.Contains(strings.ToLower(err.Error()), "unique") {
				servicehttp.WriteError(w, http.StatusConflict, "group profile already exists")
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to create group profile")
			return
		}
		servicehttp.WriteJSON(w, http.StatusCreated, map[string]any{"profile": profile})
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (d *Deps) HandleGroupProfileActions(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/group-profiles/")
	if path == r.URL.Path || path == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "group profile path not found")
		return
	}
	if d.GroupProfileStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "group profile store unavailable")
		return
	}

	parts := strings.Split(path, "/")
	profileID := strings.TrimSpace(parts[0])
	if profileID == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "group profile path not found")
		return
	}

	// POST /group-profiles/{id}/assign
	if len(parts) == 2 && parts[1] == "assign" {
		if r.Method != http.MethodPost {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if !d.EnforceRateLimit(w, r, "groupprofiles.assign", 120, time.Minute) {
			return
		}
		if _, ok, err := d.GroupProfileStore.GetGroupProfile(profileID); err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load group profile")
			return
		} else if !ok {
			servicehttp.WriteError(w, http.StatusNotFound, "group profile not found")
			return
		}
		var req struct {
			GroupID    string `json:"group_id"`
			AssignedBy string `json:"assigned_by,omitempty"`
		}
		if err := d.DecodeJSONBody(w, r, &req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid assign payload")
			return
		}
		groupID := strings.TrimSpace(req.GroupID)
		if groupID == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "group_id is required")
			return
		}
		assignment, err := d.GroupProfileStore.AssignGroupProfile(groupID, profileID, strings.TrimSpace(req.AssignedBy))
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to assign group profile")
			return
		}
		servicehttp.WriteJSON(w, http.StatusCreated, map[string]any{"assignment": assignment})
		return
	}

	// GET /group-profiles/{id}/drift?group_id={groupID}
	if len(parts) == 2 && parts[1] == "drift" {
		if r.Method != http.MethodGet {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		groupID := shared.GroupIDQueryParam(r)
		if groupID == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "group_id query parameter is required")
			return
		}
		checks, err := d.GroupProfileStore.ListDriftChecks(groupID, parseLimit(r, 50))
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list drift checks")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"drift_checks": checks})
		return
	}

	if len(parts) > 1 {
		servicehttp.WriteError(w, http.StatusNotFound, "unknown group profile action")
		return
	}

	// GET/PATCH/DELETE /group-profiles/{id}
	switch r.Method {
	case http.MethodGet:
		profile, ok, err := d.GroupProfileStore.GetGroupProfile(profileID)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load group profile")
			return
		}
		if !ok {
			servicehttp.WriteError(w, http.StatusNotFound, "group profile not found")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"profile": profile})
	case http.MethodPatch, http.MethodPut:
		if !d.EnforceRateLimit(w, r, "groupprofiles.update", 180, time.Minute) {
			return
		}
		var req groupprofiles.UpdateProfileRequest
		if err := d.DecodeJSONBody(w, r, &req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid group profile payload")
			return
		}
		if req.Name != nil {
			trimmed := strings.TrimSpace(*req.Name)
			if trimmed == "" {
				servicehttp.WriteError(w, http.StatusBadRequest, "name cannot be empty")
				return
			}
			req.Name = &trimmed
		}
		updated, err := d.GroupProfileStore.UpdateGroupProfile(profileID, req)
		if err != nil {
			if err == groupprofiles.ErrProfileNotFound {
				servicehttp.WriteError(w, http.StatusNotFound, "group profile not found")
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to update group profile")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"profile": updated})
	case http.MethodDelete:
		if err := d.GroupProfileStore.DeleteGroupProfile(profileID); err != nil {
			if err == groupprofiles.ErrProfileNotFound {
				servicehttp.WriteError(w, http.StatusNotFound, "group profile not found")
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to delete group profile")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"status": "deleted"})
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

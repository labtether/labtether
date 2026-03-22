package groupfeatures

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/groupmaintenance"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/servicehttp"
)

// HandleGroupMaintenanceWindowsCollection handles GET/POST /api/v1/groups/:id/maintenance-windows.
func (d *Deps) HandleGroupMaintenanceWindowsCollection(w http.ResponseWriter, r *http.Request, groupID string) {
	switch r.Method {
	case http.MethodGet:
		limit := shared.ParseLimit(r, 50)
		var activeAt *time.Time
		if strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("active")), "true") {
			now := time.Now().UTC()
			activeAt = &now
		}
		windows, err := d.GroupMaintenanceStore.ListGroupMaintenanceWindows(groupID, activeAt, limit)
		if err != nil {
			if errors.Is(err, groupmaintenance.ErrGroupNotFound) {
				servicehttp.WriteError(w, http.StatusNotFound, "group not found")
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list maintenance windows")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
			"group_id": groupID,
			"windows":  windows,
		})
	case http.MethodPost:
		var req groupmaintenance.CreateMaintenanceWindowRequest
		if err := shared.DecodeJSONBody(w, r, &req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid maintenance window payload")
			return
		}
		req.Name = strings.TrimSpace(req.Name)
		if req.Name == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "name is required")
			return
		}
		if req.StartAt.IsZero() || req.EndAt.IsZero() || !req.EndAt.After(req.StartAt) {
			servicehttp.WriteError(w, http.StatusBadRequest, "start_at and end_at are required and end_at must be after start_at")
			return
		}

		window, err := d.GroupMaintenanceStore.CreateGroupMaintenanceWindow(groupID, req)
		if err != nil {
			if errors.Is(err, groupmaintenance.ErrGroupNotFound) {
				servicehttp.WriteError(w, http.StatusNotFound, "group not found")
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to create maintenance window")
			return
		}
		servicehttp.WriteJSON(w, http.StatusCreated, map[string]any{"window": window})
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// HandleGroupMaintenanceWindowActions handles GET/PUT/PATCH/DELETE on a single window.
func (d *Deps) HandleGroupMaintenanceWindowActions(w http.ResponseWriter, r *http.Request, groupID, windowID string) {
	switch r.Method {
	case http.MethodGet:
		window, ok, err := d.GroupMaintenanceStore.GetGroupMaintenanceWindow(groupID, windowID)
		if err != nil {
			if errors.Is(err, groupmaintenance.ErrGroupNotFound) {
				servicehttp.WriteError(w, http.StatusNotFound, "group not found")
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load maintenance window")
			return
		}
		if !ok {
			servicehttp.WriteError(w, http.StatusNotFound, "maintenance window not found")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"window": window})
	case http.MethodPut, http.MethodPatch:
		var req groupmaintenance.UpdateMaintenanceWindowRequest
		if err := shared.DecodeJSONBody(w, r, &req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid maintenance window payload")
			return
		}
		req.Name = strings.TrimSpace(req.Name)
		if req.Name == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "name is required")
			return
		}
		if req.StartAt.IsZero() || req.EndAt.IsZero() || !req.EndAt.After(req.StartAt) {
			servicehttp.WriteError(w, http.StatusBadRequest, "start_at and end_at are required and end_at must be after start_at")
			return
		}

		window, err := d.GroupMaintenanceStore.UpdateGroupMaintenanceWindow(groupID, windowID, req)
		if err != nil {
			if errors.Is(err, groupmaintenance.ErrGroupNotFound) {
				servicehttp.WriteError(w, http.StatusNotFound, "group not found")
				return
			}
			if errors.Is(err, groupmaintenance.ErrMaintenanceWindowNotFound) {
				servicehttp.WriteError(w, http.StatusNotFound, "maintenance window not found")
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to update maintenance window")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"window": window})
	case http.MethodDelete:
		err := d.GroupMaintenanceStore.DeleteGroupMaintenanceWindow(groupID, windowID)
		if err != nil {
			if errors.Is(err, groupmaintenance.ErrGroupNotFound) {
				servicehttp.WriteError(w, http.StatusNotFound, "group not found")
				return
			}
			if errors.Is(err, groupmaintenance.ErrMaintenanceWindowNotFound) {
				servicehttp.WriteError(w, http.StatusNotFound, "maintenance window not found")
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to delete maintenance window")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
			"status":    "deleted",
			"group_id":  groupID,
			"window_id": windowID,
		})
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// ResolveGroupIDForAction returns the group ID for an action execution request.
// It first attempts to resolve the target asset's group, then falls back to
// an explicit group_id param.
func (d *Deps) ResolveGroupIDForAction(req actions.ExecuteRequest) (string, error) {
	target := strings.TrimSpace(req.Target)
	if target != "" && d.AssetStore != nil {
		assetEntry, ok, err := d.AssetStore.GetAsset(target)
		if err != nil {
			return "", err
		}
		if ok {
			groupID := strings.TrimSpace(assetEntry.GroupID)
			if groupID != "" {
				return groupID, nil
			}
		}
	}

	if req.Params != nil {
		groupID := strings.TrimSpace(req.Params["group_id"])
		if groupID != "" && d.GroupStore != nil {
			_, ok, err := d.GroupStore.GetGroup(groupID)
			if err != nil {
				return "", err
			}
			if ok {
				return groupID, nil
			}
		}
	}
	return "", nil
}

// ResolveGroupIDsForTargets maps a slice of target IDs (asset IDs or group IDs)
// to the set of group IDs they belong to.
func (d *Deps) ResolveGroupIDsForTargets(targets []string) (map[string]struct{}, error) {
	groupIDs := make(map[string]struct{}, 8)
	if len(targets) == 0 {
		return groupIDs, nil
	}

	for _, target := range targets {
		trimmed := strings.TrimSpace(target)
		if trimmed == "" {
			continue
		}
		if d.AssetStore != nil {
			assetEntry, ok, err := d.AssetStore.GetAsset(trimmed)
			if err != nil {
				return nil, err
			}
			if ok {
				groupID := strings.TrimSpace(assetEntry.GroupID)
				if groupID != "" {
					groupIDs[groupID] = struct{}{}
				}
				continue
			}
		}
		if d.GroupStore != nil {
			if _, ok, err := d.GroupStore.GetGroup(trimmed); err == nil && ok {
				groupIDs[trimmed] = struct{}{}
			} else if err != nil {
				return nil, err
			}
		}
	}
	return groupIDs, nil
}

// EvaluateGuardrails returns the active maintenance guardrails for a group at
// the given time. It is the canonical entry point used by handler packages
// that receive this method as an injected function field.
func (d *Deps) EvaluateGuardrails(groupID string, at time.Time) (GroupMaintenanceGuardrails, error) {
	return d.GroupGuardrails(groupID, at)
}

// GroupGuardrails returns the active maintenance guardrails for a group at the given time.
func (d *Deps) GroupGuardrails(groupID string, at time.Time) (GroupMaintenanceGuardrails, error) {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return GroupMaintenanceGuardrails{}, nil
	}
	if d.GroupMaintenanceStore == nil {
		return GroupMaintenanceGuardrails{}, nil
	}
	if at.IsZero() {
		at = time.Now().UTC()
	} else {
		at = at.UTC()
	}

	windows, err := d.GroupMaintenanceStore.ListGroupMaintenanceWindows(groupID, &at, 50)
	if err != nil {
		return GroupMaintenanceGuardrails{}, err
	}

	out := GroupMaintenanceGuardrails{
		GroupID:       groupID,
		ActiveWindows: windows,
	}
	for _, window := range windows {
		out.SuppressAlerts = out.SuppressAlerts || window.SuppressAlerts
		out.BlockActions = out.BlockActions || window.BlockActions
		out.BlockUpdates = out.BlockUpdates || window.BlockUpdates
	}
	return out, nil
}

package resources

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/groups"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/servicehttp"
)

func (d *Deps) HandleGroups(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/groups" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if d.GroupStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "group store unavailable")
		return
	}

	switch r.Method {
	case http.MethodGet:
		format := strings.TrimSpace(r.URL.Query().Get("format"))
		if format == "tree" {
			tree, err := d.GroupStore.GetGroupTree()
			if err != nil {
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load group tree")
				return
			}
			servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"tree": tree})
			return
		}

		allGroups, err := d.GroupStore.ListGroups()
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list groups")
			return
		}

		if r.URL.Query().Get("has_location") == "true" {
			filtered := make([]groups.Group, 0, len(allGroups))
			for _, g := range allGroups {
				if g.Timezone != "" || g.Location != "" {
					filtered = append(filtered, g)
				}
			}
			allGroups = filtered
		}

		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"groups": allGroups})

	case http.MethodPost:
		if !d.EnforceRateLimit(w, r, "groups.create", 120, time.Minute) {
			return
		}

		var req groups.CreateRequest
		if err := d.DecodeJSONBody(w, r, &req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid group payload")
			return
		}

		req.Name = strings.TrimSpace(req.Name)
		if req.Name == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "name is required")
			return
		}

		created, err := d.GroupStore.CreateGroup(req)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to create group")
			return
		}
		servicehttp.WriteJSON(w, http.StatusCreated, map[string]any{"group": created})

	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (d *Deps) HandleGroupActions(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/groups/")
	if path == r.URL.Path || path == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "group path not found")
		return
	}
	if d.GroupStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "group store unavailable")
		return
	}

	parts := strings.Split(path, "/")
	groupID := strings.TrimSpace(parts[0])
	if groupID == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "group path not found")
		return
	}

	// Sub-path routing for sub-resources is handled by the caller (cmd/labtether),
	// which dispatches timeline/reliability/maintenance to other handlers.
	// This handler covers only: /groups/{id}, /groups/{id}/move, /groups/{id}/reorder.

	if len(parts) == 2 {
		switch parts[1] {
		case "move":
			if r.Method != http.MethodPut {
				servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			d.handleGroupMove(w, r, groupID)
			return
		case "reorder":
			if r.Method != http.MethodPut {
				servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			d.handleGroupReorder(w, r, groupID)
			return
		}
		// Other sub-paths (timeline, reliability, maintenance-windows) are handled
		// by the caller in cmd/labtether, which has the full deps for those.
		servicehttp.WriteError(w, http.StatusNotFound, "unknown group action")
		return
	}

	if len(parts) != 1 {
		servicehttp.WriteError(w, http.StatusNotFound, "unknown group action")
		return
	}

	switch r.Method {
	case http.MethodGet:
		g, ok, err := d.GroupStore.GetGroup(groupID)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load group")
			return
		}
		if !ok {
			servicehttp.WriteError(w, http.StatusNotFound, "group not found")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"group": g})

	case http.MethodPut, http.MethodPatch:
		if !d.EnforceRateLimit(w, r, "groups.update", 180, time.Minute) {
			return
		}

		var req groups.UpdateRequest
		if err := d.DecodeJSONBody(w, r, &req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid group payload")
			return
		}

		updated, err := d.GroupStore.UpdateGroup(groupID, req)
		if err != nil {
			if errors.Is(err, persistence.ErrNotFound) {
				servicehttp.WriteError(w, http.StatusNotFound, "group not found")
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to update group")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"group": updated})

	case http.MethodDelete:
		if err := d.GroupStore.DeleteGroup(groupID); err != nil {
			if errors.Is(err, persistence.ErrNotFound) {
				servicehttp.WriteError(w, http.StatusNotFound, "group not found")
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to delete group")
			return
		}

		// Unlink any assets still assigned to the deleted group.
		unlinked := 0
		if d.AssetStore != nil {
			allAssets, listErr := d.AssetStore.ListAssets()
			if listErr == nil {
				emptyGroupID := ""
				for _, asset := range allAssets {
					if strings.TrimSpace(asset.GroupID) == groupID {
						if _, updateErr := d.AssetStore.UpdateAsset(asset.ID, assets.UpdateRequest{GroupID: &emptyGroupID}); updateErr == nil {
							unlinked++
						}
					}
				}
			}
		}

		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"deleted": true, "id": groupID, "unlinked_assets": unlinked})

	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (d *Deps) handleGroupMove(w http.ResponseWriter, r *http.Request, groupID string) {
	if !d.EnforceRateLimit(w, r, "groups.move", 60, time.Minute) {
		return
	}

	var body struct {
		ParentGroupID string `json:"parent_group_id"`
	}
	if err := d.DecodeJSONBody(w, r, &body); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid move payload")
		return
	}

	newParentID := strings.TrimSpace(body.ParentGroupID)

	if newParentID == groupID {
		servicehttp.WriteError(w, http.StatusConflict, "a group cannot be its own parent")
		return
	}

	if newParentID != "" {
		isAnc, err := d.GroupStore.IsAncestor(groupID, newParentID)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to check ancestry")
			return
		}
		if isAnc {
			servicehttp.WriteError(w, http.StatusConflict, "moving this group under the target would create a cycle")
			return
		}
	}

	parentPtr := &newParentID
	updated, err := d.GroupStore.UpdateGroup(groupID, groups.UpdateRequest{
		ParentGroupID: parentPtr,
	})
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			servicehttp.WriteError(w, http.StatusNotFound, "group not found")
			return
		}
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to move group")
		return
	}
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"group": updated})
}

func (d *Deps) handleGroupReorder(w http.ResponseWriter, r *http.Request, groupID string) {
	if !d.EnforceRateLimit(w, r, "groups.reorder", 180, time.Minute) {
		return
	}

	var body struct {
		SortOrder int `json:"sort_order"`
	}
	if err := d.DecodeJSONBody(w, r, &body); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid reorder payload")
		return
	}

	updated, err := d.GroupStore.UpdateGroup(groupID, groups.UpdateRequest{
		SortOrder: &body.SortOrder,
	})
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			servicehttp.WriteError(w, http.StatusNotFound, "group not found")
			return
		}
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to reorder group")
		return
	}
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"group": updated})
}

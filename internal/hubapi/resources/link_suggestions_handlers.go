package resources

import (
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/dependencies"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/servicehttp"
)

const (
	maxBulkMoveRawAssetIDs    = 256
	maxBulkMoveUniqueAssetIDs = 64
	maxBulkMoveAssetIDLength  = 255
)

// HandleLinkSuggestions handles GET /links/suggestions.
func (d *Deps) HandleLinkSuggestions(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/links/suggestions" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if d.LinkSuggestionStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "link suggestion store unavailable")
		return
	}

	switch r.Method {
	case http.MethodGet:
		suggestions, err := d.LinkSuggestionStore.ListPendingLinkSuggestions()
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list link suggestions")
			return
		}
		if shared.HasAssetRestriction(r.Context()) {
			filtered := make([]persistence.LinkSuggestion, 0, len(suggestions))
			for _, suggestion := range suggestions {
				if shared.AllAssetsAllowed(r.Context(), suggestion.SourceAssetID, suggestion.TargetAssetID) {
					filtered = append(filtered, suggestion)
				}
			}
			suggestions = filtered
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"suggestions": suggestions})
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// HandleLinkSuggestionActions handles PUT /links/suggestions/{id}.
func (d *Deps) HandleLinkSuggestionActions(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/links/suggestions/")
	if path == r.URL.Path || path == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "suggestion path not found")
		return
	}
	if d.LinkSuggestionStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "link suggestion store unavailable")
		return
	}

	id := strings.TrimSpace(strings.Trim(path, "/"))
	if id == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "suggestion id required")
		return
	}

	switch r.Method {
	case http.MethodPut:
		suggestion, ok, err := d.LinkSuggestionStore.GetLinkSuggestion(id)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load link suggestion")
			return
		}
		if !ok {
			servicehttp.WriteError(w, http.StatusNotFound, "link suggestion not found")
			return
		}
		if !requireAssetAccess(w, r, suggestion.SourceAssetID, suggestion.TargetAssetID) {
			return
		}
		if !d.EnforceRateLimit(w, r, "links.suggestions.resolve", 120, time.Minute) {
			return
		}
		var req struct {
			Status string `json:"status"`
		}
		if err := d.DecodeJSONBody(w, r, &req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid request payload")
			return
		}
		status := strings.TrimSpace(req.Status)
		if status != "accepted" && status != "dismissed" {
			servicehttp.WriteError(w, http.StatusBadRequest, "status must be 'accepted' or 'dismissed'")
			return
		}

		if err := d.LinkSuggestionStore.ResolveLinkSuggestion(id, status, "user"); err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to resolve link suggestion")
			return
		}

		// If accepted, create a 'contains' dependency edge between source and target.
		if status == "accepted" && d.DependencyStore != nil {
			sourceID := strings.TrimSpace(suggestion.SourceAssetID)
			targetID := strings.TrimSpace(suggestion.TargetAssetID)
			if sourceID != "" && targetID != "" {
				_, _ = d.DependencyStore.CreateAssetDependency(dependencies.CreateDependencyRequest{
					SourceAssetID:    sourceID,
					TargetAssetID:    targetID,
					RelationshipType: dependencies.RelationshipContains,
					Direction:        dependencies.DirectionDownstream,
					Criticality:      dependencies.CriticalityMedium,
				})
			}
		}

		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"status": "resolved"})
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// HandleManualLink handles POST /links/manual.
func (d *Deps) HandleManualLink(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/links/manual" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if d.DependencyStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "dependency store unavailable")
		return
	}

	switch r.Method {
	case http.MethodPost:
		if !d.EnforceRateLimit(w, r, "links.manual.create", 120, time.Minute) {
			return
		}
		var req struct {
			SourceID string `json:"source_id"`
			TargetID string `json:"target_id"`
		}
		if err := d.DecodeJSONBody(w, r, &req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid request payload")
			return
		}
		sourceID := strings.TrimSpace(req.SourceID)
		targetID := strings.TrimSpace(req.TargetID)
		if sourceID == "" || targetID == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "source_id and target_id are required")
			return
		}
		if sourceID == targetID {
			servicehttp.WriteError(w, http.StatusBadRequest, "source_id and target_id must be different")
			return
		}
		if !requireAssetAccess(w, r, sourceID, targetID) {
			return
		}

		dep, err := d.DependencyStore.CreateAssetDependency(dependencies.CreateDependencyRequest{
			SourceAssetID:    sourceID,
			TargetAssetID:    targetID,
			RelationshipType: dependencies.RelationshipContains,
			Direction:        dependencies.DirectionDownstream,
			Criticality:      dependencies.CriticalityMedium,
		})
		if err != nil {
			if err == dependencies.ErrDuplicateDependency {
				servicehttp.WriteError(w, http.StatusConflict, "link already exists")
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to create link")
			return
		}
		servicehttp.WriteJSON(w, http.StatusCreated, map[string]any{"dependency": dep})
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// HandleAssetBulkMove handles PUT /assets/bulk/move.
func (d *Deps) HandleAssetBulkMove(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/assets/bulk/move" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}

	switch r.Method {
	case http.MethodPut:
		if !d.EnforceRateLimit(w, r, "assets.bulk.move", 60, time.Minute) {
			return
		}
		var req struct {
			AssetIDs []string `json:"asset_ids"`
			GroupID  string   `json:"group_id"`
		}
		if err := d.DecodeJSONBody(w, r, &req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid request payload")
			return
		}
		if len(req.AssetIDs) == 0 {
			servicehttp.WriteError(w, http.StatusBadRequest, "asset_ids is required")
			return
		}
		if len(req.AssetIDs) > maxBulkMoveRawAssetIDs {
			servicehttp.WriteError(w, http.StatusBadRequest, "asset_ids exceeds maximum request size")
			return
		}
		assetIDs := make([]string, 0, min(len(req.AssetIDs), maxBulkMoveUniqueAssetIDs))
		seenAssetIDs := make(map[string]struct{}, len(req.AssetIDs))
		for _, rawAssetID := range req.AssetIDs {
			assetID := strings.TrimSpace(rawAssetID)
			if assetID == "" || len(assetID) > maxBulkMoveAssetIDLength {
				servicehttp.WriteError(w, http.StatusBadRequest, "asset_ids must contain valid non-empty identifiers")
				return
			}
			if _, duplicate := seenAssetIDs[assetID]; duplicate {
				continue
			}
			seenAssetIDs[assetID] = struct{}{}
			if len(assetIDs) >= maxBulkMoveUniqueAssetIDs {
				servicehttp.WriteError(w, http.StatusBadRequest, "asset_ids exceeds maximum of 64 unique assets")
				return
			}
			assetIDs = append(assetIDs, assetID)
		}
		groupID := strings.TrimSpace(req.GroupID)
		if !requireAssetAccess(w, r, assetIDs...) {
			return
		}

		if groupID != "" {
			if d.GroupStore == nil {
				servicehttp.WriteError(w, http.StatusServiceUnavailable, "group store unavailable")
				return
			}
			_, ok, err := d.GroupStore.GetGroup(groupID)
			if err != nil {
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to validate group")
				return
			}
			if !ok {
				servicehttp.WriteError(w, http.StatusNotFound, "group not found")
				return
			}
			if !d.requireGroupAccess(w, r, groupID) {
				return
			}
		}

		updated := 0
		for _, assetID := range assetIDs {
			gid := groupID
			_, err := d.AssetStore.UpdateAsset(assetID, assets.UpdateRequest{GroupID: &gid})
			if err != nil {
				continue
			}
			updated++
		}

		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
			"updated": updated,
		})
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

package logspkg

import (
	"errors"
	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/servicehttp"
	"net/http"
	"strings"
)

// HandleLogViews handles GET and POST /logs/views.
func (d *Deps) HandleLogViews(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/logs/views" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}

	switch r.Method {
	case http.MethodGet:
		views, err := d.LogStore.ListViews(apiv2.PrincipalActorID(r.Context()), shared.ParseLimit(r, 50))
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list log views")
			return
		}
		filtered := views[:0]
		for _, view := range views {
			if savedViewAllowed(r.Context(), view.AssetID) {
				filtered = append(filtered, view)
			}
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"views": filtered})
	case http.MethodPost:
		var req logs.SavedViewRequest
		if err := shared.DecodeJSONBody(w, r, &req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid log view payload")
			return
		}
		if strings.TrimSpace(req.ID) != "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "id must not be provided when creating a log view")
			return
		}
		req.Name = strings.TrimSpace(req.Name)
		if req.Name == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "name is required")
			return
		}
		if !requireSavedViewAccess(w, r, req.AssetID) {
			return
		}

		view, err := d.LogStore.SaveView(apiv2.PrincipalActorID(r.Context()), req)
		if err != nil {
			if errors.Is(err, persistence.ErrAlreadyExists) {
				servicehttp.WriteError(w, http.StatusConflict, "log view already exists")
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to save log view")
			return
		}
		servicehttp.WriteJSON(w, http.StatusCreated, map[string]any{"view": view})
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// HandleLogViewActions handles GET, PUT/PATCH, and DELETE /logs/views/{id}.
func (d *Deps) HandleLogViewActions(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/logs/views/")
	if path == r.URL.Path || path == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "log view path not found")
		return
	}

	viewID := strings.TrimSpace(path)
	if strings.Contains(viewID, "/") {
		servicehttp.WriteError(w, http.StatusNotFound, "unknown log view action")
		return
	}
	existing, ok, err := d.LogStore.GetView(apiv2.PrincipalActorID(r.Context()), viewID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load log view")
		return
	}
	if !ok {
		servicehttp.WriteError(w, http.StatusNotFound, "log view not found")
		return
	}
	if !requireSavedViewAccess(w, r, existing.AssetID) {
		return
	}

	switch r.Method {
	case http.MethodGet:
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"view": existing})
	case http.MethodPut, http.MethodPatch:
		var req logs.SavedViewRequest
		if err := shared.DecodeJSONBody(w, r, &req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid log view payload")
			return
		}
		if bodyID := strings.TrimSpace(req.ID); bodyID != "" && bodyID != viewID {
			servicehttp.WriteError(w, http.StatusBadRequest, "body id must match the requested log view")
			return
		}
		req.Name = strings.TrimSpace(req.Name)
		if req.Name == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "name is required")
			return
		}
		if !requireSavedViewAccess(w, r, req.AssetID) {
			return
		}
		view, err := d.LogStore.UpdateView(apiv2.PrincipalActorID(r.Context()), viewID, req)
		if err != nil {
			if errors.Is(err, persistence.ErrNotFound) {
				servicehttp.WriteError(w, http.StatusNotFound, "log view not found")
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to update log view")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"view": view})
	case http.MethodDelete:
		if err := d.LogStore.DeleteView(apiv2.PrincipalActorID(r.Context()), viewID); err != nil {
			if errors.Is(err, persistence.ErrNotFound) {
				servicehttp.WriteError(w, http.StatusNotFound, "log view not found")
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to delete log view")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"deleted": true, "view_id": viewID})
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

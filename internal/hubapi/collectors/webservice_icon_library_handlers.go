package collectors

import (
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/servicehttp"
)

type WebServiceIconLibraryEntry struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	DataURL   string `json:"data_url"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

type webServiceIconLibraryCreateRequest struct {
	Name    string `json:"name"`
	DataURL string `json:"data_url"`
}

type webServiceIconLibraryRenameRequest struct {
	Name string `json:"name"`
}

func (d *Deps) HandleWebServiceIconLibrary(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/v1/services/web/icon-library" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if d.RuntimeStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "runtime settings unavailable")
		return
	}

	switch r.Method {
	case http.MethodGet:
		icons, err := d.loadWebServiceIconLibrary()
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load service icon library")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"icons": icons})
	case http.MethodPost, http.MethodPut:
		var req webServiceIconLibraryCreateRequest
		if err := shared.DecodeJSONBody(w, r, &req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid icon payload")
			return
		}

		name := normalizeCustomServiceIconDisplayName(req.Name)
		if name == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "name is required")
			return
		}
		if len(name) > maxCustomServiceIconDisplayNameChars {
			servicehttp.WriteError(w, http.StatusBadRequest, "name is too long")
			return
		}
		dataURL := strings.TrimSpace(req.DataURL)
		if err := validateCustomServiceIconDataURL(dataURL); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}

		icons, err := d.loadWebServiceIconLibrary()
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load service icon library")
			return
		}
		if len(icons) >= maxCustomServiceIconLibraryItems {
			servicehttp.WriteError(w, http.StatusBadRequest, "service icon library is full")
			return
		}

		now := time.Now().UTC().Format(time.RFC3339)
		entry := WebServiceIconLibraryEntry{
			ID:        generateCustomServiceIconID(name),
			Name:      name,
			DataURL:   dataURL,
			CreatedAt: now,
			UpdatedAt: now,
		}
		icons = append(icons, entry)
		if err := d.saveWebServiceIconLibrary(icons); err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to save service icon library")
			return
		}
		servicehttp.WriteJSON(w, http.StatusCreated, map[string]any{
			"icon":  entry,
			"icons": icons,
		})
	case http.MethodPatch:
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		if id == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "id is required")
			return
		}

		var req webServiceIconLibraryRenameRequest
		if err := shared.DecodeJSONBody(w, r, &req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid icon payload")
			return
		}
		name := normalizeCustomServiceIconDisplayName(req.Name)
		if name == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "name is required")
			return
		}
		if len(name) > maxCustomServiceIconDisplayNameChars {
			servicehttp.WriteError(w, http.StatusBadRequest, "name is too long")
			return
		}

		icons, err := d.loadWebServiceIconLibrary()
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load service icon library")
			return
		}

		now := time.Now().UTC().Format(time.RFC3339)
		found := false
		var updated WebServiceIconLibraryEntry
		for index := range icons {
			if icons[index].ID != id {
				continue
			}
			icons[index].Name = name
			icons[index].UpdatedAt = now
			updated = icons[index]
			found = true
			break
		}
		if !found {
			servicehttp.WriteError(w, http.StatusNotFound, "icon not found")
			return
		}
		if err := d.saveWebServiceIconLibrary(icons); err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to save service icon library")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
			"icon":  updated,
			"icons": icons,
		})
	case http.MethodDelete:
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		if id == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "id is required")
			return
		}

		icons, err := d.loadWebServiceIconLibrary()
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load service icon library")
			return
		}
		updated := make([]WebServiceIconLibraryEntry, 0, len(icons))
		removed := false
		for _, icon := range icons {
			if icon.ID == id {
				removed = true
				continue
			}
			updated = append(updated, icon)
		}
		if !removed {
			servicehttp.WriteError(w, http.StatusNotFound, "icon not found")
			return
		}
		if err := d.saveWebServiceIconLibrary(updated); err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to save service icon library")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

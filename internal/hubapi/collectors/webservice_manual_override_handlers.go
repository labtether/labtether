package collectors

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/labtether/labtether/internal/connectors/webservice"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/servicehttp"
	"github.com/labtether/labtether/internal/hubapi/shared"
)

type webServiceManualRequest struct {
	HostAssetID string            `json:"host_asset_id"`
	Name        string            `json:"name"`
	Category    string            `json:"category"`
	URL         string            `json:"url"`
	IconKey     string            `json:"icon_key,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type webServiceManualPatchRequest struct {
	HostAssetID *string           `json:"host_asset_id,omitempty"`
	Name        *string           `json:"name,omitempty"`
	Category    *string           `json:"category,omitempty"`
	URL         *string           `json:"url,omitempty"`
	IconKey     *string           `json:"icon_key,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type webServiceOverrideRequest struct {
	HostAssetID      string `json:"host_asset_id"`
	ServiceID        string `json:"service_id"`
	NameOverride     string `json:"name_override,omitempty"`
	CategoryOverride string `json:"category_override,omitempty"`
	URLOverride      string `json:"url_override,omitempty"`
	IconKeyOverride  string `json:"icon_key_override,omitempty"`
	TagsOverride     string `json:"tags_override,omitempty"`
	Hidden           bool   `json:"hidden"`
}

// HandleWebServiceManual handles GET/POST /api/v1/services/web/manual.
func (d *Deps) HandleWebServiceManual(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/v1/services/web/manual" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if d.WebServiceCoordinator == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "web service coordinator unavailable")
		return
	}

	switch r.Method {
	case http.MethodGet:
		hostFilter := strings.TrimSpace(r.URL.Query().Get("host"))
		items, err := d.WebServiceCoordinator.ListManualServices(hostFilter)
		if err != nil {
			if errors.Is(err, webservice.ErrStoreUnavailable) {
				servicehttp.WriteError(w, http.StatusServiceUnavailable, "web service store unavailable")
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list manual services")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"services": items})
	case http.MethodPost:
		var req webServiceManualRequest
		if err := shared.DecodeJSONBody(w, r, &req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid manual service payload")
			return
		}
		manual, err := d.buildManualService(req, "")
		if err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		saved, err := d.WebServiceCoordinator.SaveManualService(manual)
		if err != nil {
			if errors.Is(err, webservice.ErrStoreUnavailable) {
				servicehttp.WriteError(w, http.StatusServiceUnavailable, "web service store unavailable")
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to save manual service")
			return
		}
		servicehttp.WriteJSON(w, http.StatusCreated, map[string]any{"service": saved})
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// HandleWebServiceManualActions handles PATCH/DELETE /api/v1/services/web/manual/{id}.
func (d *Deps) HandleWebServiceManualActions(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/v1/services/web/manual/"))
	if trimmed == "" || trimmed == r.URL.Path {
		servicehttp.WriteError(w, http.StatusNotFound, "manual service path not found")
		return
	}
	parts := strings.Split(strings.Trim(trimmed, "/"), "/")
	if len(parts) != 1 {
		servicehttp.WriteError(w, http.StatusNotFound, "manual service path not found")
		return
	}
	id := strings.TrimSpace(parts[0])
	if id == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "manual service path not found")
		return
	}
	if d.WebServiceCoordinator == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "web service coordinator unavailable")
		return
	}

	switch r.Method {
	case http.MethodPatch, http.MethodPut:
		existing, ok, err := d.WebServiceCoordinator.GetManualService(id)
		if err != nil {
			if errors.Is(err, webservice.ErrStoreUnavailable) {
				servicehttp.WriteError(w, http.StatusServiceUnavailable, "web service store unavailable")
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load manual service")
			return
		}
		if !ok {
			servicehttp.WriteError(w, http.StatusNotFound, "manual service not found")
			return
		}

		var patchReq webServiceManualPatchRequest
		if err := shared.DecodeJSONBody(w, r, &patchReq); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid manual service payload")
			return
		}

		mergedReq := webServiceManualRequest{
			HostAssetID: existing.HostAssetID,
			Name:        existing.Name,
			Category:    existing.Category,
			URL:         existing.URL,
			IconKey:     existing.IconKey,
			Metadata:    existing.Metadata,
		}
		if patchReq.HostAssetID != nil {
			mergedReq.HostAssetID = strings.TrimSpace(*patchReq.HostAssetID)
		}
		if patchReq.Name != nil {
			mergedReq.Name = strings.TrimSpace(*patchReq.Name)
		}
		if patchReq.Category != nil {
			mergedReq.Category = strings.TrimSpace(*patchReq.Category)
		}
		if patchReq.URL != nil {
			mergedReq.URL = strings.TrimSpace(*patchReq.URL)
		}
		if patchReq.IconKey != nil {
			mergedReq.IconKey = strings.TrimSpace(*patchReq.IconKey)
		}
		if patchReq.Metadata != nil {
			mergedReq.Metadata = patchReq.Metadata
		}

		item, err := d.buildManualService(mergedReq, id)
		if err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		saved, err := d.WebServiceCoordinator.SaveManualService(item)
		if err != nil {
			if errors.Is(err, webservice.ErrStoreUnavailable) {
				servicehttp.WriteError(w, http.StatusServiceUnavailable, "web service store unavailable")
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to update manual service")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"service": saved})
	case http.MethodDelete:
		if err := d.WebServiceCoordinator.DeleteManualService(id); err != nil {
			switch {
			case errors.Is(err, webservice.ErrStoreUnavailable):
				servicehttp.WriteError(w, http.StatusServiceUnavailable, "web service store unavailable")
			case errors.Is(err, persistence.ErrNotFound):
				servicehttp.WriteError(w, http.StatusNotFound, "manual service not found")
			default:
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to delete manual service")
			}
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// HandleWebServiceOverrides handles GET/POST/DELETE /api/v1/services/web/overrides.
func (d *Deps) HandleWebServiceOverrides(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/v1/services/web/overrides" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if d.WebServiceCoordinator == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "web service coordinator unavailable")
		return
	}

	switch r.Method {
	case http.MethodGet:
		hostFilter := strings.TrimSpace(r.URL.Query().Get("host"))
		items, err := d.WebServiceCoordinator.ListOverrides(hostFilter)
		if err != nil {
			if errors.Is(err, webservice.ErrStoreUnavailable) {
				servicehttp.WriteError(w, http.StatusServiceUnavailable, "web service store unavailable")
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list service overrides")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"overrides": items})
	case http.MethodPost, http.MethodPut:
		var req webServiceOverrideRequest
		if err := shared.DecodeJSONBody(w, r, &req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid service override payload")
			return
		}
		override, err := d.buildWebServiceOverride(req)
		if err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		saved, err := d.WebServiceCoordinator.SaveOverride(override)
		if err != nil {
			if errors.Is(err, webservice.ErrStoreUnavailable) {
				servicehttp.WriteError(w, http.StatusServiceUnavailable, "web service store unavailable")
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to save service override")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"override": saved})
	case http.MethodDelete:
		host := strings.TrimSpace(r.URL.Query().Get("host"))
		serviceID := strings.TrimSpace(r.URL.Query().Get("service_id"))
		if host == "" || serviceID == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "host and service_id are required")
			return
		}
		if err := d.WebServiceCoordinator.DeleteOverride(host, serviceID); err != nil {
			switch {
			case errors.Is(err, webservice.ErrStoreUnavailable):
				servicehttp.WriteError(w, http.StatusServiceUnavailable, "web service store unavailable")
			case errors.Is(err, persistence.ErrNotFound):
				servicehttp.WriteError(w, http.StatusNotFound, "service override not found")
			default:
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to delete service override")
			}
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (d *Deps) buildManualService(req webServiceManualRequest, existingID string) (persistence.WebServiceManual, error) {
	host := strings.TrimSpace(req.HostAssetID)
	name := strings.TrimSpace(req.Name)
	category := strings.TrimSpace(req.Category)
	rawURL := strings.TrimSpace(req.URL)
	iconKey := strings.TrimSpace(req.IconKey)

	if host != "" && d.AssetStore != nil {
		if _, ok, err := d.AssetStore.GetAsset(host); err != nil {
			return persistence.WebServiceManual{}, errors.New("failed to validate host asset")
		} else if !ok {
			return persistence.WebServiceManual{}, errors.New("host asset does not exist")
		}
	}
	if name == "" {
		return persistence.WebServiceManual{}, errors.New("name is required")
	}
	if category == "" {
		category = "Other"
	}
	if rawURL == "" {
		return persistence.WebServiceManual{}, errors.New("url is required")
	}
	if err := validateWebServiceURL(rawURL); err != nil {
		return persistence.WebServiceManual{}, err
	}

	return persistence.WebServiceManual{
		ID:          strings.TrimSpace(existingID),
		HostAssetID: host,
		Name:        name,
		Category:    category,
		URL:         rawURL,
		IconKey:     iconKey,
		Metadata:    req.Metadata,
	}, nil
}

func (d *Deps) buildWebServiceOverride(req webServiceOverrideRequest) (persistence.WebServiceOverride, error) {
	host := strings.TrimSpace(req.HostAssetID)
	serviceID := strings.TrimSpace(req.ServiceID)
	if host == "" {
		return persistence.WebServiceOverride{}, errors.New("host_asset_id is required")
	}
	if serviceID == "" {
		return persistence.WebServiceOverride{}, errors.New("service_id is required")
	}
	if req.URLOverride != "" {
		if err := validateWebServiceURL(req.URLOverride); err != nil {
			return persistence.WebServiceOverride{}, err
		}
	}

	return persistence.WebServiceOverride{
		HostAssetID:      host,
		ServiceID:        serviceID,
		NameOverride:     strings.TrimSpace(req.NameOverride),
		CategoryOverride: strings.TrimSpace(req.CategoryOverride),
		URLOverride:      strings.TrimSpace(req.URLOverride),
		IconKeyOverride:  strings.TrimSpace(req.IconKeyOverride),
		TagsOverride:     strings.TrimSpace(req.TagsOverride),
		Hidden:           req.Hidden,
	}, nil
}

func validateWebServiceURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return errors.New("invalid url")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("url must use http or https")
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return errors.New("url host is required")
	}
	return nil
}

package webhookspkg

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/webhooks"
)

const (
	maxWebhookNameLength = 120
	maxWebhookEventCount = 64
)

// HandleV2Webhooks routes collection-level webhook requests (GET list, POST create).
func (d *Deps) HandleV2Webhooks(w http.ResponseWriter, r *http.Request) {
	scope := "webhooks:read"
	if apiv2.IsMutatingMethod(r.Method) {
		scope = "webhooks:write"
	}
	if !apiv2.ScopeCheck(apiv2.ScopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	switch r.Method {
	case http.MethodGet:
		d.HandleV2WebhookList(w, r)
	case http.MethodPost:
		d.HandleV2WebhookCreate(w, r)
	default:
		apiv2.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	}
}

// HandleV2WebhookActions routes per-resource webhook requests (GET, PATCH, DELETE).
func (d *Deps) HandleV2WebhookActions(w http.ResponseWriter, r *http.Request) {
	scope := "webhooks:read"
	if apiv2.IsMutatingMethod(r.Method) {
		scope = "webhooks:write"
	}
	if !apiv2.ScopeCheck(apiv2.ScopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/v2/webhooks/")
	if id == "" || id == r.URL.Path {
		apiv2.WriteError(w, http.StatusNotFound, "not_found", "webhook id required")
		return
	}
	switch r.Method {
	case http.MethodGet:
		d.HandleV2WebhookGet(w, r, id)
	case http.MethodPatch:
		d.HandleV2WebhookUpdate(w, r, id)
	case http.MethodDelete:
		d.HandleV2WebhookDelete(w, r, id)
	default:
		apiv2.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	}
}

// HandleV2WebhookCreate creates a new webhook.
func (d *Deps) HandleV2WebhookCreate(w http.ResponseWriter, r *http.Request) {
	if d.WebhookStore == nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "not_configured", "webhook store not configured")
		return
	}

	var req webhooks.CreateRequest
	if err := shared.DecodeJSONBody(w, r, &req); err != nil {
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len(req.Name) > maxWebhookNameLength {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "name is required (max 120 chars)")
		return
	}
	req.URL = strings.TrimSpace(req.URL)
	if req.URL == "" {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "url is required")
		return
	}
	// Validate URL scheme.
	parsedURL, parseErr := url.Parse(req.URL)
	if parseErr != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "url must use http or https scheme")
		return
	}
	if len(req.Events) > maxWebhookEventCount {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "too many events")
		return
	}
	if req.Events == nil {
		req.Events = []string{}
	}

	now := time.Now().UTC()
	wh := webhooks.Webhook{
		ID:        idgen.New("wh"),
		Name:      req.Name,
		URL:       req.URL,
		Secret:    req.Secret,
		Events:    req.Events,
		Enabled:   true,
		CreatedBy: apiv2.PrincipalActorID(r.Context()),
		CreatedAt: now,
	}

	if err := d.WebhookStore.CreateWebhook(r.Context(), wh); err != nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to save webhook")
		return
	}

	shared.AppendAuditEventBestEffort(d.AuditStore, audit.Event{
		Type:      "webhook.created",
		ActorID:   apiv2.PrincipalActorID(r.Context()),
		Target:    wh.ID,
		Details:   map[string]any{"name": wh.Name, "url": wh.URL, "events": wh.Events},
		Timestamp: now,
	}, "webhook created: "+wh.Name)

	apiv2.WriteJSON(w, http.StatusCreated, wh)
}

// HandleV2WebhookList returns all webhooks.
func (d *Deps) HandleV2WebhookList(w http.ResponseWriter, r *http.Request) {
	if d.WebhookStore == nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "not_configured", "webhook store not configured")
		return
	}
	list, err := d.WebhookStore.ListWebhooks(r.Context())
	if err != nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to list webhooks")
		return
	}
	if list == nil {
		list = []webhooks.Webhook{}
	}
	apiv2.WriteJSON(w, http.StatusOK, list)
}

// HandleV2WebhookGet returns a single webhook by ID.
func (d *Deps) HandleV2WebhookGet(w http.ResponseWriter, r *http.Request, id string) {
	if d.WebhookStore == nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "not_configured", "webhook store not configured")
		return
	}
	wh, ok, err := d.WebhookStore.GetWebhook(r.Context(), id)
	if err != nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to get webhook")
		return
	}
	if !ok {
		apiv2.WriteError(w, http.StatusNotFound, "not_found", "webhook not found")
		return
	}
	apiv2.WriteJSON(w, http.StatusOK, wh)
}

// HandleV2WebhookUpdate applies a partial update to an existing webhook.
func (d *Deps) HandleV2WebhookUpdate(w http.ResponseWriter, r *http.Request, id string) {
	if d.WebhookStore == nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "not_configured", "webhook store not configured")
		return
	}

	var req struct {
		Name    *string   `json:"name,omitempty"`
		URL     *string   `json:"url,omitempty"`
		Events  *[]string `json:"events,omitempty"`
		Enabled *bool     `json:"enabled,omitempty"`
	}
	if err := shared.DecodeJSONBody(w, r, &req); err != nil {
		return
	}

	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		if trimmed == "" || len(trimmed) > maxWebhookNameLength {
			apiv2.WriteError(w, http.StatusBadRequest, "validation", "invalid name")
			return
		}
		req.Name = &trimmed
	}
	if req.URL != nil {
		trimmed := strings.TrimSpace(*req.URL)
		if trimmed == "" {
			apiv2.WriteError(w, http.StatusBadRequest, "validation", "url cannot be empty")
			return
		}
		// Validate URL scheme.
		parsedURL, parseErr := url.Parse(trimmed)
		if parseErr != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
			apiv2.WriteError(w, http.StatusBadRequest, "validation", "url must use http or https scheme")
			return
		}
		req.URL = &trimmed
	}

	if err := d.WebhookStore.UpdateWebhook(r.Context(), id, req.Name, req.URL, req.Events, req.Enabled); err != nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to update webhook")
		return
	}
	apiv2.WriteJSON(w, http.StatusOK, map[string]any{"status": "updated"})
}

// HandleV2WebhookDelete removes a webhook by ID.
func (d *Deps) HandleV2WebhookDelete(w http.ResponseWriter, r *http.Request, id string) {
	if d.WebhookStore == nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "not_configured", "webhook store not configured")
		return
	}
	if err := d.WebhookStore.DeleteWebhook(r.Context(), id); err != nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to delete webhook")
		return
	}

	shared.AppendAuditEventBestEffort(d.AuditStore, audit.Event{
		Type:      "webhook.deleted",
		ActorID:   apiv2.PrincipalActorID(r.Context()),
		Target:    id,
		Timestamp: time.Now().UTC(),
	}, "webhook deleted: "+id)

	apiv2.WriteJSON(w, http.StatusOK, map[string]any{"status": "deleted"})
}

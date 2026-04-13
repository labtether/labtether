package webhookspkg

import (
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/persistence"
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

// HandleV2WebhookActions routes per-resource webhook requests (GET, PUT/PATCH, DELETE).
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
	case http.MethodPut:
		d.HandleV2WebhookUpdate(w, r, id)
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
	if err := validateWebhookURL(req.URL); err != nil {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", err.Error())
		return
	}
	events, err := normalizeWebhookEvents(req.Events)
	if err != nil {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", err.Error())
		return
	}

	webhookID := idgen.New("wh")
	secretCiphertext, err := d.encryptWebhookSecret(req.Secret, webhookID)
	if err != nil {
		apiv2.WriteError(w, http.StatusServiceUnavailable, "not_configured", err.Error())
		return
	}

	now := time.Now().UTC()
	wh := webhooks.Webhook{
		ID:               webhookID,
		Name:             req.Name,
		URL:              req.URL,
		SecretCiphertext: secretCiphertext,
		Events:           events,
		Enabled:          true,
		CreatedBy:        apiv2.PrincipalActorID(r.Context()),
		CreatedAt:        now,
	}

	if err := d.WebhookStore.CreateWebhook(r.Context(), wh); err != nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to save webhook")
		return
	}
	d.invalidateWebhookCache()

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

	var req webhooks.UpdateRequest
	if err := shared.DecodeJSONBody(w, r, &req); err != nil {
		return
	}
	if req.Name == nil && req.URL == nil && req.Secret == nil && req.Events == nil && req.Enabled == nil {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "at least one field must be provided")
		return
	}

	existing, ok, err := d.WebhookStore.GetWebhook(r.Context(), id)
	if err != nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to get webhook")
		return
	}
	if !ok {
		apiv2.WriteError(w, http.StatusNotFound, "not_found", "webhook not found")
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
		if err := validateWebhookURL(trimmed); err != nil {
			apiv2.WriteError(w, http.StatusBadRequest, "validation", err.Error())
			return
		}
		req.URL = &trimmed
	}
	if req.Events != nil {
		normalizedEvents, err := normalizeWebhookEvents(*req.Events)
		if err != nil {
			apiv2.WriteError(w, http.StatusBadRequest, "validation", err.Error())
			return
		}
		req.Events = &normalizedEvents
	}

	updated := existing
	if req.Name != nil {
		updated.Name = *req.Name
	}
	if req.URL != nil {
		updated.URL = *req.URL
	}
	if req.Events != nil {
		updated.Events = *req.Events
	}
	if req.Enabled != nil {
		updated.Enabled = *req.Enabled
	}
	if req.Secret != nil {
		secretCiphertext, err := d.encryptWebhookSecret(*req.Secret, id)
		if err != nil {
			apiv2.WriteError(w, http.StatusServiceUnavailable, "not_configured", err.Error())
			return
		}
		updated.Secret = ""
		updated.SecretCiphertext = secretCiphertext
	}

	if err := d.WebhookStore.UpdateWebhook(r.Context(), updated); err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			apiv2.WriteError(w, http.StatusNotFound, "not_found", "webhook not found")
			return
		}
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to update webhook")
		return
	}
	d.invalidateWebhookCache()
	apiv2.WriteJSON(w, http.StatusOK, updated)
}

// HandleV2WebhookDelete removes a webhook by ID.
func (d *Deps) HandleV2WebhookDelete(w http.ResponseWriter, r *http.Request, id string) {
	if d.WebhookStore == nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "not_configured", "webhook store not configured")
		return
	}
	if _, ok, err := d.WebhookStore.GetWebhook(r.Context(), id); err != nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to get webhook")
		return
	} else if !ok {
		apiv2.WriteError(w, http.StatusNotFound, "not_found", "webhook not found")
		return
	}
	if err := d.WebhookStore.DeleteWebhook(r.Context(), id); err != nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to delete webhook")
		return
	}
	d.invalidateWebhookCache()

	shared.AppendAuditEventBestEffort(d.AuditStore, audit.Event{
		Type:      "webhook.deleted",
		ActorID:   apiv2.PrincipalActorID(r.Context()),
		Target:    id,
		Timestamp: time.Now().UTC(),
	}, "webhook deleted: "+id)

	apiv2.WriteJSON(w, http.StatusOK, map[string]any{"status": "deleted"})
}

func validateWebhookURL(raw string) error {
	parsedURL, parseErr := url.Parse(strings.TrimSpace(raw))
	if parseErr != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return errors.New("url must use http or https scheme")
	}
	if strings.TrimSpace(parsedURL.Host) == "" {
		return errors.New("url host is required")
	}
	return nil
}

func normalizeWebhookEvents(events []string) ([]string, error) {
	if len(events) == 0 {
		return []string{}, nil
	}

	normalized := make([]string, 0, len(events))
	seen := make(map[string]struct{}, len(events))
	for _, event := range events {
		event = strings.TrimSpace(event)
		if event == "" {
			return nil, errors.New("event names cannot be blank")
		}
		if _, ok := seen[event]; ok {
			continue
		}
		seen[event] = struct{}{}
		normalized = append(normalized, event)
	}
	if len(normalized) > maxWebhookEventCount {
		return nil, errors.New("too many events")
	}
	slices.Sort(normalized)
	return normalized, nil
}

func (d *Deps) encryptWebhookSecret(secret string, webhookID string) (string, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return "", nil
	}
	if d.SecretsManager == nil {
		return "", errors.New("webhook secret storage is not configured")
	}
	return d.SecretsManager.EncryptString(secret, webhookID)
}

func (d *Deps) invalidateWebhookCache() {
	if d.InvalidateWebhookCache != nil {
		d.InvalidateWebhookCache()
	}
}

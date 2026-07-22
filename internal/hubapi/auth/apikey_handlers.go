package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/apikeys"
	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/audit"
	internalauth "github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/idgen"
)

const (
	maxAPIKeyNameLength = 120
	maxAPIKeyScopeCount = 64
	maxAPIKeyAssetCount = 200
	maxAPIKeyAssetIDLen = 255
)

func normalizeAPIKeyAllowedAssets(values []string) ([]string, error) {
	if len(values) > maxAPIKeyAssetCount {
		return nil, fmt.Errorf("too many allowed_assets")
	}
	if len(values) == 0 {
		return nil, nil
	}

	normalized := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		assetID := strings.TrimSpace(value)
		if assetID == "" || len(assetID) > maxAPIKeyAssetIDLen {
			return nil, fmt.Errorf("allowed_assets contains an invalid asset id")
		}
		for _, char := range assetID {
			if char < 0x20 || char == 0x7f {
				return nil, fmt.Errorf("allowed_assets contains an invalid asset id")
			}
		}
		if _, duplicate := seen[assetID]; duplicate {
			return nil, fmt.Errorf("allowed_assets must not contain duplicates")
		}
		seen[assetID] = struct{}{}
		normalized = append(normalized, assetID)
	}
	return normalized, nil
}

// HandleAPIKeys handles GET/POST /api/v2/keys.
func (d *Deps) HandleAPIKeys(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		d.handleAPIKeysList(w, r)
	case http.MethodPost:
		d.handleAPIKeyCreate(w, r)
	default:
		apiv2.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	}
}

// HandleAPIKeyActions handles GET/PATCH/DELETE /api/v2/keys/{id}.
func (d *Deps) HandleAPIKeyActions(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v2/keys/")
	if id == "" || id == r.URL.Path {
		apiv2.WriteError(w, http.StatusNotFound, "not_found", "key id required")
		return
	}
	switch r.Method {
	case http.MethodGet:
		d.handleAPIKeyGet(w, r, id)
	case http.MethodPatch:
		d.handleAPIKeyUpdate(w, r, id)
	case http.MethodDelete:
		d.handleAPIKeyDelete(w, r, id)
	default:
		apiv2.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	}
}

func (d *Deps) handleAPIKeyCreate(w http.ResponseWriter, r *http.Request) {
	if d.APIKeyStore == nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "not_configured", "api keys not configured")
		return
	}

	var req apikeys.CreateKeyRequest
	if err := d.decodeJSONBodyV2(w, r, &req); err != nil {
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len(req.Name) > maxAPIKeyNameLength {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "name is required (max 120 chars)")
		return
	}
	if !internalauth.IsValidRole(req.Role) {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "invalid role")
		return
	}
	if req.Role == internalauth.RoleOwner {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "owner role is reserved")
		return
	}
	if err := apikeys.ValidateScopes(req.Scopes); err != nil {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", err.Error())
		return
	}
	if len(req.Scopes) > maxAPIKeyScopeCount {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "too many scopes")
		return
	}
	allowedAssets, err := normalizeAPIKeyAllowedAssets(req.AllowedAssets)
	if err != nil {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", err.Error())
		return
	}
	req.AllowedAssets = allowedAssets

	generated, err := apikeys.GenerateKey()
	if err != nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "key generation failed")
		return
	}

	now := time.Now().UTC()
	key := apikeys.APIKey{
		ID:            idgen.New("key"),
		Name:          req.Name,
		Prefix:        generated.Prefix,
		SecretHash:    generated.Hash,
		Role:          internalauth.NormalizeRole(req.Role),
		Scopes:        req.Scopes,
		AllowedAssets: req.AllowedAssets,
		ExpiresAt:     req.ExpiresAt,
		CreatedBy:     d.actorIDFromContext(r.Context()),
		CreatedAt:     now,
	}

	if err := d.APIKeyStore.CreateAPIKey(r.Context(), key); err != nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to save key")
		return
	}

	if d.AppendAuditEventBestEffort != nil {
		d.AppendAuditEventBestEffort(audit.Event{
			Type:      "api_key.created",
			ActorID:   d.actorIDFromContext(r.Context()),
			Target:    key.ID,
			Details:   map[string]any{"name": key.Name, "role": key.Role, "scopes": key.Scopes},
			Timestamp: now,
		}, "api key created: "+key.Name)
	}

	apiv2.WriteJSON(w, http.StatusCreated, map[string]any{
		"id":             key.ID,
		"name":           key.Name,
		"prefix":         key.Prefix,
		"raw_key":        generated.Raw,
		"role":           key.Role,
		"scopes":         key.Scopes,
		"allowed_assets": key.AllowedAssets,
		"expires_at":     key.ExpiresAt,
		"created_at":     key.CreatedAt,
	})
}

func (d *Deps) handleAPIKeysList(w http.ResponseWriter, r *http.Request) {
	if d.APIKeyStore == nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "not_configured", "api keys not configured")
		return
	}
	keys, err := d.APIKeyStore.ListAPIKeys(r.Context())
	if err != nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to list keys")
		return
	}
	infos := make([]apikeys.KeyInfo, len(keys))
	for i, k := range keys {
		infos[i] = apikeys.KeyInfo{
			ID:            k.ID,
			Name:          k.Name,
			Prefix:        k.Prefix,
			Role:          k.Role,
			Scopes:        k.Scopes,
			AllowedAssets: k.AllowedAssets,
			ExpiresAt:     k.ExpiresAt,
			CreatedBy:     k.CreatedBy,
			CreatedAt:     k.CreatedAt,
			LastUsedAt:    k.LastUsedAt,
		}
	}
	apiv2.WriteList(w, http.StatusOK, infos, len(infos), 1, len(infos))
}

func (d *Deps) handleAPIKeyGet(w http.ResponseWriter, r *http.Request, id string) {
	if d.APIKeyStore == nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "not_configured", "api keys not configured")
		return
	}
	key, ok, err := d.APIKeyStore.GetAPIKey(r.Context(), id)
	if err != nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to get key")
		return
	}
	if !ok {
		apiv2.WriteError(w, http.StatusNotFound, "not_found", "key not found")
		return
	}
	apiv2.WriteJSON(w, http.StatusOK, apikeys.KeyInfo{
		ID:            key.ID,
		Name:          key.Name,
		Prefix:        key.Prefix,
		Role:          key.Role,
		Scopes:        key.Scopes,
		AllowedAssets: key.AllowedAssets,
		ExpiresAt:     key.ExpiresAt,
		CreatedBy:     key.CreatedBy,
		CreatedAt:     key.CreatedAt,
		LastUsedAt:    key.LastUsedAt,
	})
}

func (d *Deps) handleAPIKeyUpdate(w http.ResponseWriter, r *http.Request, id string) {
	if d.APIKeyStore == nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "not_configured", "api keys not configured")
		return
	}

	var req struct {
		Name          *string         `json:"name,omitempty"`
		Role          *string         `json:"role,omitempty"`
		Scopes        *[]string       `json:"scopes,omitempty"`
		AllowedAssets *[]string       `json:"allowed_assets,omitempty"`
		ExpiresAt     json.RawMessage `json:"expires_at,omitempty"`
	}
	if err := d.decodeJSONBodyV2(w, r, &req); err != nil {
		return
	}
	if req.Role != nil {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "api key role cannot be changed")
		return
	}

	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		if trimmed == "" || len(trimmed) > maxAPIKeyNameLength {
			apiv2.WriteError(w, http.StatusBadRequest, "validation", "invalid name")
			return
		}
		req.Name = &trimmed
	}
	if req.Scopes != nil {
		if err := apikeys.ValidateScopes(*req.Scopes); err != nil {
			apiv2.WriteError(w, http.StatusBadRequest, "validation", err.Error())
			return
		}
		if len(*req.Scopes) > maxAPIKeyScopeCount {
			apiv2.WriteError(w, http.StatusBadRequest, "validation", "too many scopes")
			return
		}
	}
	if req.AllowedAssets != nil {
		normalized, err := normalizeAPIKeyAllowedAssets(*req.AllowedAssets)
		if err != nil {
			apiv2.WriteError(w, http.StatusBadRequest, "validation", err.Error())
			return
		}
		req.AllowedAssets = &normalized
	}
	var expiresAtUpdate **time.Time
	if len(req.ExpiresAt) > 0 {
		if strings.TrimSpace(string(req.ExpiresAt)) == "null" {
			var clearExpiry *time.Time
			expiresAtUpdate = &clearExpiry
		} else {
			var expiresAt time.Time
			if err := json.Unmarshal(req.ExpiresAt, &expiresAt); err != nil || expiresAt.IsZero() {
				apiv2.WriteError(w, http.StatusBadRequest, "validation", "expires_at must be an RFC3339 timestamp or null")
				return
			}
			expiresAt = expiresAt.UTC()
			expiresAtValue := &expiresAt
			expiresAtUpdate = &expiresAtValue
		}
	}

	if err := d.APIKeyStore.UpdateAPIKey(r.Context(), id, req.Name, req.Scopes, req.AllowedAssets, expiresAtUpdate); err != nil {
		if errors.Is(err, apikeys.ErrNotFound) {
			apiv2.WriteError(w, http.StatusNotFound, "not_found", "key not found")
			return
		}
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to update key")
		return
	}
	apiv2.WriteJSON(w, http.StatusOK, map[string]any{"status": "updated"})
}

func (d *Deps) handleAPIKeyDelete(w http.ResponseWriter, r *http.Request, id string) {
	if d.APIKeyStore == nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "not_configured", "api keys not configured")
		return
	}
	if err := d.APIKeyStore.DeleteAPIKey(r.Context(), id); err != nil {
		apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to delete key")
		return
	}

	if d.AppendAuditEventBestEffort != nil {
		d.AppendAuditEventBestEffort(audit.Event{
			Type:      "api_key.deleted",
			ActorID:   d.actorIDFromContext(r.Context()),
			Target:    id,
			Timestamp: time.Now().UTC(),
		}, "api key deleted: "+id)
	}

	apiv2.WriteJSON(w, http.StatusOK, map[string]any{"status": "deleted"})
}

// actorIDFromContext extracts the actor ID from the context using the injected
// UserIDFromContext function.
func (d *Deps) actorIDFromContext(ctx context.Context) string {
	if d.UserIDFromContext != nil {
		return d.UserIDFromContext(ctx)
	}
	return ""
}

// decodeJSONBodyV2 decodes the request body and writes an apiv2 error on failure.
func (d *Deps) decodeJSONBodyV2(w http.ResponseWriter, r *http.Request, dst any) error {
	if err := d.decodeJSONBody(w, r, dst); err != nil {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "invalid request payload")
		return err
	}
	return nil
}

// decodeJSONBody decodes the request body using the shared helper.
func (d *Deps) decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any) error {
	return shared.DecodeJSONBody(w, r, dst)
}

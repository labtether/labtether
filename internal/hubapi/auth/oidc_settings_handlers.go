package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/servicehttp"
)

const (
	oidcSettingsKey    = "oidc"
	oidcSecretMask     = "********************"
	oidcApplyRateLimit = 5
)

// Local env helpers — mirrors internal/hubapi/shared but kept local so the
// auth sub-package does not take a hard import on shared for env reads alone.

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envOrDefaultBool(key string, fallback bool) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return parsed
}

// oidcDBSettings is the shape stored as JSON in system_settings.
type oidcDBSettings struct {
	Enabled            *bool    `json:"enabled,omitempty"`
	IssuerURL          string   `json:"issuer_url,omitempty"`
	ClientID           string   `json:"client_id,omitempty"`
	ClientSecret       string   `json:"client_secret,omitempty"` // #nosec G117 -- Settings payload intentionally carries runtime OIDC secret material.
	Scopes             []string `json:"scopes,omitempty"`
	RoleClaim          string   `json:"role_claim,omitempty"`
	DefaultRole        string   `json:"default_role,omitempty"`
	DisplayName        string   `json:"display_name,omitempty"`
	AdminRoleValues    []string `json:"admin_role_values,omitempty"`
	OperatorRoleValues []string `json:"operator_role_values,omitempty"`
	AutoProvision      *bool    `json:"auto_provision,omitempty"`
}

// oidcSettingsPutRequest is the JSON body accepted by PUT /settings/oidc.
type oidcSettingsPutRequest struct {
	Enabled            bool   `json:"enabled"`
	IssuerURL          string `json:"issuer_url"`
	ClientID           string `json:"client_id"`
	ClientSecret       string `json:"client_secret"` // #nosec G117 -- Settings payload intentionally carries runtime OIDC secret material.
	Scopes             string `json:"scopes"`
	RoleClaim          string `json:"role_claim"`
	DefaultRole        string `json:"default_role"`
	DisplayName        string `json:"display_name"`
	AdminRoleValues    string `json:"admin_role_values"`
	OperatorRoleValues string `json:"operator_role_values"`
	AutoProvision      bool   `json:"auto_provision"`
}

// maskSecret returns the mask sentinel if the value is non-empty.
func maskSecret(v string) string {
	if v == "" {
		return ""
	}
	return oidcSecretMask
}

// csvJoin joins a slice into a comma-separated string.
func csvJoin(parts []string) string {
	return strings.Join(parts, ",")
}

// csvSplit splits a comma-separated string into a trimmed, non-empty slice.
func csvSplit(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// loadOIDCDBSettings reads the oidc settings blob from the DB; returns a
// zero-value struct (no error) when the key is absent.
func (d *Deps) loadOIDCDBSettings(ctx context.Context) (oidcDBSettings, error) {
	var out oidcDBSettings
	if d.SettingsStore == nil {
		return out, nil
	}
	raw, ok, err := d.SettingsStore.GetSystemSetting(ctx, oidcSettingsKey)
	if err != nil || !ok {
		return out, err
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, err
	}
	return out, nil
}

// mergeOIDCSettings computes effective OIDC settings by layering DB values
// over hard defaults, with env vars taking highest precedence.
// It also returns a per-field source map ("env", "db", or "default").
func mergeOIDCSettings(db oidcDBSettings) (auth.OIDCSettings, map[string]string) {
	sources := make(map[string]string)

	resolve := func(field, envKey, dbVal, def string) string {
		if ev := strings.TrimSpace(os.Getenv(envKey)); ev != "" {
			sources[field] = "env"
			return ev
		}
		if dbVal != "" {
			sources[field] = "db"
			return dbVal
		}
		sources[field] = "default"
		return def
	}

	resolveBool := func(field, envKey string, dbPtr *bool, def bool) bool {
		if ev := strings.TrimSpace(os.Getenv(envKey)); ev != "" {
			if parsed, err := strconv.ParseBool(ev); err == nil {
				sources[field] = "env"
				return parsed
			}
		}
		if dbPtr != nil {
			sources[field] = "db"
			return *dbPtr
		}
		sources[field] = "default"
		return def
	}

	resolveSlice := func(field, envKey string, dbSlice []string, def []string) []string {
		if ev := strings.TrimSpace(os.Getenv(envKey)); ev != "" {
			sources[field] = "env"
			return csvSplit(ev)
		}
		if len(dbSlice) > 0 {
			sources[field] = "db"
			return dbSlice
		}
		sources[field] = "default"
		return def
	}

	issuerURL := strings.TrimSpace(envOrDefault("LABTETHER_OIDC_ISSUER_URL", ""))
	clientID := strings.TrimSpace(envOrDefault("LABTETHER_OIDC_CLIENT_ID", ""))

	// enabled: env > db > infer from issuer+client_id presence > false
	enabled := resolveBool("enabled", "LABTETHER_OIDC_ENABLED", db.Enabled,
		issuerURL != "" || clientID != "")

	s := auth.OIDCSettings{
		Enabled:   enabled,
		IssuerURL: resolve("issuer_url", "LABTETHER_OIDC_ISSUER_URL", db.IssuerURL, ""),
		ClientID:  resolve("client_id", "LABTETHER_OIDC_CLIENT_ID", db.ClientID, ""),
		ClientSecret: resolve("client_secret", "LABTETHER_OIDC_CLIENT_SECRET",
			db.ClientSecret, ""),
		RoleClaim:   resolve("role_claim", "LABTETHER_OIDC_ROLE_CLAIM", db.RoleClaim, "labtether_role"),
		DisplayName: resolve("display_name", "LABTETHER_OIDC_DISPLAY_NAME", db.DisplayName, "Single Sign-On"),
		DefaultRole: resolve("default_role", "LABTETHER_OIDC_DEFAULT_ROLE", db.DefaultRole, auth.RoleViewer),
		Scopes: resolveSlice("scopes", "LABTETHER_OIDC_SCOPES", db.Scopes,
			[]string{"openid", "profile", "email"}),
		AdminRoleValues: resolveSlice("admin_role_values", "LABTETHER_OIDC_ADMIN_ROLES",
			db.AdminRoleValues, []string{"admin"}),
		OperatorRoleValues: resolveSlice("operator_role_values", "LABTETHER_OIDC_OPERATOR_ROLES",
			db.OperatorRoleValues, []string{"operator"}),
	}

	// auto_provision is not part of OIDCSettings but track its source
	resolveBool("auto_provision", "LABTETHER_OIDC_AUTO_PROVISION", db.AutoProvision, true)

	return s, sources
}

// HandleOIDCSettingsGet handles GET /settings/oidc.
func (d *Deps) HandleOIDCSettingsGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	db, err := d.loadOIDCDBSettings(r.Context())
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load oidc settings")
		return
	}

	merged, sources := mergeOIDCSettings(db)

	// Compute auto_provision effective value separately (not in OIDCSettings).
	autoProvision := envOrDefaultBool("LABTETHER_OIDC_AUTO_PROVISION",
		func() bool {
			if db.AutoProvision != nil {
				return *db.AutoProvision
			}
			return true
		}(),
	)

	// Active provider state from ref.
	activeProvider, _ := d.OIDCRef.Get()

	activeIssuer := ""
	if activeProvider != nil {
		activeIssuer = activeProvider.IssuerURL()
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"enabled":              merged.Enabled,
		"issuer_url":           merged.IssuerURL,
		"client_id":            merged.ClientID,
		"client_secret":        maskSecret(merged.ClientSecret),
		"scopes":               csvJoin(merged.Scopes),
		"role_claim":           merged.RoleClaim,
		"default_role":         merged.DefaultRole,
		"display_name":         merged.DisplayName,
		"admin_role_values":    csvJoin(merged.AdminRoleValues),
		"operator_role_values": csvJoin(merged.OperatorRoleValues),
		"auto_provision":       autoProvision,
		"sources":              sources,
		"active":               activeProvider != nil,
		"active_issuer":        activeIssuer,
	})
}

// HandleOIDCSettingsPut handles PUT /settings/oidc.
func (d *Deps) HandleOIDCSettingsPut(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req oidcSettingsPutRequest
	if err := shared.DecodeJSONBody(w, r, &req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid oidc settings payload")
		return
	}

	// Validate required fields when enabled.
	if req.Enabled {
		issuerURL := strings.TrimSpace(req.IssuerURL)
		if issuerURL == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "issuer_url is required when oidc is enabled")
			return
		}
		if _, err := url.ParseRequestURI(issuerURL); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "issuer_url must be a valid URL")
			return
		}
		if strings.TrimSpace(req.ClientID) == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "client_id is required when oidc is enabled")
			return
		}
	}

	// Validate default_role if non-empty: must be a valid role, not owner.
	if req.DefaultRole != "" {
		role := strings.ToLower(strings.TrimSpace(req.DefaultRole))
		if !auth.IsValidRole(role) {
			servicehttp.WriteError(w, http.StatusBadRequest, "default_role must be admin, operator, or viewer")
			return
		}
		if role == auth.RoleOwner {
			servicehttp.WriteError(w, http.StatusBadRequest, "default_role cannot be owner")
			return
		}
		req.DefaultRole = role
	}

	// Load existing DB settings so we can preserve the secret if masked value sent.
	existing, err := d.loadOIDCDBSettings(r.Context())
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load existing oidc settings")
		return
	}

	// If the caller sent back the mask, keep the existing secret unchanged.
	clientSecret := req.ClientSecret
	if clientSecret == oidcSecretMask {
		clientSecret = existing.ClientSecret
	}

	enabled := req.Enabled
	autoProvision := req.AutoProvision

	updated := oidcDBSettings{
		Enabled:            &enabled,
		IssuerURL:          strings.TrimSpace(req.IssuerURL),
		ClientID:           strings.TrimSpace(req.ClientID),
		ClientSecret:       strings.TrimSpace(clientSecret),
		Scopes:             csvSplit(req.Scopes),
		RoleClaim:          strings.TrimSpace(req.RoleClaim),
		DefaultRole:        strings.TrimSpace(req.DefaultRole),
		DisplayName:        strings.TrimSpace(req.DisplayName),
		AdminRoleValues:    csvSplit(req.AdminRoleValues),
		OperatorRoleValues: csvSplit(req.OperatorRoleValues),
		AutoProvision:      &autoProvision,
	}

	raw, err := json.Marshal(updated) // #nosec G117 -- Marshaling runtime OIDC settings for persistence, not embedding a hardcoded secret.
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to marshal oidc settings")
		return
	}
	if d.SettingsStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "settings store unavailable")
		return
	}
	if err := d.SettingsStore.PutSystemSetting(r.Context(), oidcSettingsKey, raw); err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to save oidc settings")
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"enabled":              enabled,
		"issuer_url":           updated.IssuerURL,
		"client_id":            updated.ClientID,
		"client_secret":        maskSecret(updated.ClientSecret),
		"scopes":               csvJoin(updated.Scopes),
		"role_claim":           updated.RoleClaim,
		"default_role":         updated.DefaultRole,
		"display_name":         updated.DisplayName,
		"admin_role_values":    csvJoin(updated.AdminRoleValues),
		"operator_role_values": csvJoin(updated.OperatorRoleValues),
		"auto_provision":       autoProvision,
	})
}

// HandleOIDCSettingsApply handles POST /settings/oidc/apply.
// It reads the merged config (DB + env overrides), instantiates a new OIDC
// provider, and atomically swaps it into OIDCRef on success.
func (d *Deps) HandleOIDCSettingsApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if !d.EnforceRateLimit(w, r, "settings.oidc.apply", oidcApplyRateLimit, time.Minute) {
		return
	}

	db, err := d.loadOIDCDBSettings(r.Context())
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load oidc settings")
		return
	}

	merged, _ := mergeOIDCSettings(db)

	// Compute auto_provision effective value.
	autoProvision := envOrDefaultBool("LABTETHER_OIDC_AUTO_PROVISION",
		func() bool {
			if db.AutoProvision != nil {
				return *db.AutoProvision
			}
			return true
		}(),
	)

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	provider, err := auth.NewOIDCProvider(ctx, merged)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to initialize oidc provider: "+err.Error())
		return
	}

	d.OIDCRef.Swap(provider, autoProvision)

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"applied":        true,
		"enabled":        provider != nil,
		"auto_provision": autoProvision,
	})
}

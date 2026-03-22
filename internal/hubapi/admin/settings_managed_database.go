package admin

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/servicehttp"
)

const (
	managedDatabaseSettingsRoute = "/settings/managed-database"
	managedDatabaseRevealRoute   = "/settings/managed-database/reveal"

	managedDatabaseRevealRateLimitKey  = "settings.managed_database.reveal"
	managedDatabaseRevealRateLimitMax  = 10
	managedDatabaseRevealRateLimitWind = time.Minute

	// ManagedDatabaseRevealAuditType is exported so cmd/labtether tests can
	// assert against the audit event type without importing the admin package.
	ManagedDatabaseRevealAuditType    = "settings.managed_database.password_revealed"
	managedDatabaseRevealAuditWarning = "api warning: failed to append managed database password reveal audit event"
)

// ManagedDatabaseSettingsPayload is the response body for
// GET /settings/managed-database. Exported so cmd/labtether tests can
// reference it via the type alias in admin_bridge.go.
type ManagedDatabaseSettingsPayload struct {
	Managed           bool   `json:"managed"`
	Engine            string `json:"engine"`
	Host              string `json:"host,omitempty"`
	Database          string `json:"database,omitempty"`
	Username          string `json:"username,omitempty"`
	PasswordAvailable bool   `json:"password_available"`
	PasswordHint      string `json:"password_hint,omitempty"`
}

// ManagedDatabaseRevealPayload is the response body for
// POST /settings/managed-database/reveal. Exported so cmd/labtether tests can
// reference it via the type alias in admin_bridge.go.
type ManagedDatabaseRevealPayload struct {
	ManagedDatabaseSettingsPayload
	Password string `json:"password"` // #nosec G117 -- Response payload intentionally reveals runtime credential material to an authorized caller.
}

// HandleManagedDatabaseSettings handles GET /settings/managed-database.
func (d *Deps) HandleManagedDatabaseSettings(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != managedDatabaseSettingsRoute {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	payload, password, err := d.managedDatabaseSettingsPayload()
	if err != nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	payload.PasswordAvailable = payload.Managed && strings.TrimSpace(password) != ""
	if payload.PasswordAvailable {
		payload.PasswordHint = maskSecret(password)
	}
	servicehttp.WriteJSON(w, http.StatusOK, payload)
}

// HandleManagedDatabasePasswordReveal handles POST /settings/managed-database/reveal.
func (d *Deps) HandleManagedDatabasePasswordReveal(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != managedDatabaseRevealRoute {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !d.enforceRateLimit(w, r, managedDatabaseRevealRateLimitKey, managedDatabaseRevealRateLimitMax, managedDatabaseRevealRateLimitWind) {
		return
	}

	payload, password, err := d.managedDatabaseSettingsPayload()
	if err != nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	password = strings.TrimSpace(password)
	if !payload.Managed || password == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "managed database password unavailable")
		return
	}

	auditEvent := audit.NewEvent(ManagedDatabaseRevealAuditType)
	auditEvent.ActorID = d.principalActorID(r.Context())
	auditEvent.Decision = "revealed"
	auditEvent.Details = map[string]any{
		"database": payload.Database,
		"host":     payload.Host,
		"engine":   payload.Engine,
	}
	d.appendAuditEventBestEffort(auditEvent, managedDatabaseRevealAuditWarning)

	payload.PasswordAvailable = true
	payload.PasswordHint = maskSecret(password)
	servicehttp.WriteJSON(w, http.StatusOK, ManagedDatabaseRevealPayload{
		ManagedDatabaseSettingsPayload: payload,
		Password:                       password,
	})
}

func (d *Deps) managedDatabaseSettingsPayload() (ManagedDatabaseSettingsPayload, string, error) {
	var payload ManagedDatabaseSettingsPayload
	if d.InstallStateStore == nil {
		return payload, "", errManagedDatabaseSettingsUnavailable
	}

	_, secrets, _, err := d.InstallStateStore.Load()
	if err != nil {
		return payload, "", errManagedDatabaseSettingsUnavailable
	}
	password := strings.TrimSpace(secrets.PostgresPassword)
	managed := shared.EnvOrDefaultBool("LABTETHER_MANAGED_POSTGRES", false) && password != ""

	payload = ManagedDatabaseSettingsPayload{
		Managed:           managed,
		Engine:            "postgres",
		PasswordAvailable: managed,
	}

	if parsed, err := url.Parse(strings.TrimSpace(shared.EnvOrDefault("DATABASE_URL", ""))); err == nil && parsed != nil {
		payload.Host = parsed.Hostname()
		payload.Username = parsed.User.Username()
		payload.Database = strings.TrimPrefix(parsed.Path, "/")
	}

	if payload.Host == "" {
		payload.Host = "postgres"
	}
	if payload.Username == "" {
		payload.Username = "labtether"
	}
	if payload.Database == "" {
		payload.Database = "labtether"
	}

	return payload, password, nil
}

// maskSecret masks all but the first two and last two characters of a secret.
func maskSecret(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if len(raw) <= 8 {
		return strings.Repeat("*", len(raw))
	}
	return raw[:2] + strings.Repeat("*", len(raw)-4) + raw[len(raw)-2:]
}

var errManagedDatabaseSettingsUnavailable = managedDBSettingsError("managed database settings unavailable")

type managedDBSettingsError string

func (e managedDBSettingsError) Error() string {
	return string(e)
}

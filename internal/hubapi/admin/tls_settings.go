package admin

import (
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/audit"
	opspkg "github.com/labtether/labtether/internal/hubapi/operations"
	"github.com/labtether/labtether/internal/servicehttp"
)

const (
	// TLSSettingsRoute is the URL path for GET/POST/DELETE /settings/tls.
	// Exported so cmd/labtether tests can reference it via the alias in admin_bridge.go.
	TLSSettingsRoute = "/settings/tls"

	tlsSettingsUpdatedAuditType = "settings.tls.updated"
	tlsSettingsClearedAuditType = "settings.tls.cleared"
)

type tlsSettingsUpdateRequest struct {
	CertPEM string `json:"cert_pem"`
	KeyPEM  string `json:"key_pem"`
}

// TLSSettingsResponse is the response body for GET/POST/DELETE /settings/tls.
// Exported so cmd/labtether tests can reference the type via the bridge.
type TLSSettingsResponse struct {
	TLSEnabled              bool                          `json:"tls_enabled"`
	TLSMode                 string                        `json:"tls_mode"`
	TLSSource               string                        `json:"tls_source"`
	CertType                string                        `json:"cert_type"`
	CAAvailable             bool                          `json:"ca_available"`
	ActiveCertificate       opspkg.TLSCertificateMetadata `json:"active_certificate,omitempty"`
	DefaultTLSMode          string                        `json:"default_tls_mode,omitempty"`
	DefaultTLSSource        string                        `json:"default_tls_source,omitempty"`
	UploadedOverridePresent bool                          `json:"uploaded_override_present"`
	UploadedCertificate     opspkg.TLSCertificateMetadata `json:"uploaded_certificate,omitempty"`
	UploadedUpdatedAt       time.Time                     `json:"uploaded_updated_at,omitempty"`
	CanUpload               bool                          `json:"can_upload"`
	CanApplyLive            bool                          `json:"can_apply_live"`
	RestartRequired         bool                          `json:"restart_required"`
	RestartActionAvailable  bool                          `json:"restart_action_available"`
	RestartActionNote       string                        `json:"restart_action_note,omitempty"`
}

// HandleTLSSettings handles GET, POST, and DELETE /settings/tls.
func (d *Deps) HandleTLSSettings(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != TLSSettingsRoute {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	switch r.Method {
	case http.MethodGet:
		resp, err := d.buildTLSSettingsResponse(false)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, resp)
	case http.MethodPost:
		d.handleTLSSettingsUpdate(w, r)
	case http.MethodDelete:
		d.handleTLSSettingsClear(w, r)
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (d *Deps) handleTLSSettingsUpdate(w http.ResponseWriter, r *http.Request) {
	if d.RuntimeStore == nil || d.SecretsManager == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "tls management is unavailable")
		return
	}
	var req tlsSettingsUpdateRequest
	if err := d.decodeJSONBody(w, r, &req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid tls settings payload")
		return
	}
	certPEM := strings.TrimSpace(req.CertPEM)
	keyPEM := strings.TrimSpace(req.KeyPEM)
	if certPEM == "" || keyPEM == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "cert_pem and key_pem are required")
		return
	}
	if _, err := opspkg.ValidateUploadedTLSPair(certPEM, keyPEM); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	keyCiphertext, err := d.SecretsManager.EncryptString(keyPEM, opspkg.TLSOverrideAAD)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to encrypt uploaded tls key")
		return
	}
	certPath, keyPath, err := opspkg.MaterializeUploadedTLSFiles(d.DataDir, certPEM, keyPEM)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to materialize uploaded tls certificate")
		return
	}
	provider, err := opspkg.NewStaticHubCertificateProvider(certPath, keyPath)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load uploaded tls certificate")
		return
	}
	updatedAt := time.Now().UTC()
	if _, err := d.RuntimeStore.SaveRuntimeSettingOverrides(map[string]string{
		opspkg.TLSOverrideCertPEMKey:   certPEM,
		opspkg.TLSOverrideKeyCipherKey: keyCiphertext,
		opspkg.TLSOverrideUpdatedAtKey: updatedAt.Format(time.RFC3339),
	}); err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to persist uploaded tls certificate")
		return
	}

	restartRequired := false
	if d.TLSState != nil && d.TLSState.Enabled && d.TLSState.CertSwitcher != nil {
		d.TLSState.CertSwitcher.SetProvider(provider.GetCertificate)
		d.TLSState.Mode = "external"
		d.TLSState.Source = opspkg.TLSSourceUIUploaded
		d.TLSState.CertFile = certPath
		d.TLSState.KeyFile = keyPath
		d.TLSState.CACertPEM = nil
	} else {
		restartRequired = true
	}

	auditEvent := audit.NewEvent(tlsSettingsUpdatedAuditType)
	auditEvent.ActorID = d.principalActorID(r.Context())
	auditEvent.Decision = "applied"
	auditEvent.Details = map[string]any{
		"tls_source":       opspkg.TLSSourceUIUploaded,
		"restart_required": restartRequired,
	}
	d.appendAuditEventBestEffort(auditEvent, "api warning: failed to append TLS settings update audit event")

	resp, buildErr := d.buildTLSSettingsResponse(restartRequired)
	if buildErr != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, buildErr.Error())
		return
	}
	servicehttp.WriteJSON(w, http.StatusOK, resp)
}

func (d *Deps) handleTLSSettingsClear(w http.ResponseWriter, r *http.Request) {
	if d.RuntimeStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "tls management is unavailable")
		return
	}
	if err := d.RuntimeStore.DeleteRuntimeSettingOverrides([]string{
		opspkg.TLSOverrideCertPEMKey,
		opspkg.TLSOverrideKeyCipherKey,
		opspkg.TLSOverrideUpdatedAtKey,
	}); err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to clear uploaded tls certificate")
		return
	}

	restartRequired := false
	if d.TLSState != nil && d.TLSState.Source == opspkg.TLSSourceUIUploaded {
		if d.TLSState.DefaultGetCertificate != nil && d.TLSState.CertSwitcher != nil {
			d.TLSState.CertSwitcher.SetProvider(d.TLSState.DefaultGetCertificate)
			d.TLSState.Mode = d.TLSState.DefaultMode
			d.TLSState.Source = d.TLSState.DefaultSource
			d.TLSState.CertFile = d.TLSState.DefaultCertFile
			d.TLSState.KeyFile = d.TLSState.DefaultKeyFile
			d.TLSState.CACertPEM = append([]byte(nil), d.TLSState.DefaultCAPEM...)
		} else {
			restartRequired = true
		}
	}

	defaultSource := ""
	if d.TLSState != nil {
		defaultSource = d.TLSState.DefaultSource
	}
	auditEvent := audit.NewEvent(tlsSettingsClearedAuditType)
	auditEvent.ActorID = d.principalActorID(r.Context())
	auditEvent.Decision = "applied"
	auditEvent.Details = map[string]any{
		"restart_required": restartRequired,
		"restored_source":  defaultSource,
	}
	d.appendAuditEventBestEffort(auditEvent, "api warning: failed to append TLS settings clear audit event")

	resp, buildErr := d.buildTLSSettingsResponse(restartRequired)
	if buildErr != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, buildErr.Error())
		return
	}
	servicehttp.WriteJSON(w, http.StatusOK, resp)
}

// BuildTLSSettingsResponse is exported so the bridge can call it for tests.
func (d *Deps) BuildTLSSettingsResponse(restartRequired bool) (TLSSettingsResponse, error) {
	return d.buildTLSSettingsResponse(restartRequired)
}

func (d *Deps) buildTLSSettingsResponse(restartRequired bool) (TLSSettingsResponse, error) {
	activeMeta, err := d.activeTLSCertificateMetadata()
	if d.TLSState != nil && err != nil && d.TLSState.Enabled {
		return TLSSettingsResponse{}, err
	}

	canUpload := d.RuntimeStore != nil && d.SecretsManager != nil
	canApplyLive := d.TLSState != nil && d.TLSState.Enabled && d.TLSState.CertSwitcher != nil

	resp := TLSSettingsResponse{
		ActiveCertificate:      activeMeta,
		CanUpload:              canUpload,
		CanApplyLive:           canApplyLive,
		RestartRequired:        restartRequired,
		RestartActionAvailable: true,
		RestartActionNote:      "Restart asks the current hub process to exit cleanly. It comes back automatically only when Docker or another process supervisor restarts it.",
	}
	if d.TLSState != nil {
		resp.TLSEnabled = d.TLSState.Enabled
		resp.TLSMode = strings.TrimSpace(d.TLSState.Mode)
		resp.TLSSource = strings.TrimSpace(d.TLSState.Source)
		resp.CAAvailable = len(d.TLSState.CACertPEM) > 0
		resp.DefaultTLSMode = strings.TrimSpace(d.TLSState.DefaultMode)
		resp.DefaultTLSSource = strings.TrimSpace(d.TLSState.DefaultSource)
	}
	resp.CertType = d.currentTLSCertType()

	if override, ok, overrideErr := opspkg.LoadPersistedTLSOverride(d.RuntimeStore, d.SecretsManager); overrideErr == nil && ok {
		overrideMeta, metaErr := opspkg.TLSCertificateMetadataFromPEM(override.CertPEM)
		if metaErr != nil {
			return TLSSettingsResponse{}, metaErr
		}
		resp.UploadedOverridePresent = true
		resp.UploadedCertificate = overrideMeta
		resp.UploadedUpdatedAt = override.UpdatedAt
	}
	return resp, nil
}

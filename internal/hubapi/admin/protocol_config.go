package admin

import (
	"encoding/json"
	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/protocols"
	"github.com/labtether/labtether/internal/securityruntime"
	"github.com/labtether/labtether/internal/servicehttp"
	"net/http"
	"strings"
	"sync"
)

// protocolTestLocks guards against concurrent test runs for the same
// asset+protocol combination. Keys are "assetID:protocol".
var protocolTestLocks sync.Map

// HandleListProtocolConfigs handles GET /assets/{id}/protocols.
func (d *Deps) HandleListProtocolConfigs(w http.ResponseWriter, r *http.Request, assetID string) {
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !apiv2.RequireAssetAccess(w, r, assetID) {
		return
	}

	configs, err := d.DB.ListProtocolConfigs(r.Context(), assetID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list protocol configs")
		return
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"protocols": configs})
}

type protocolConfigRequest struct {
	Protocol            string          `json:"protocol"`
	Host                string          `json:"host"`
	Port                int             `json:"port"`
	Username            string          `json:"username"`
	CredentialProfileID string          `json:"credential_profile_id"`
	Enabled             *bool           `json:"enabled,omitempty"`
	Config              json.RawMessage `json:"config"`
}

// HandleCreateProtocolConfig handles POST /assets/{id}/protocols.
func (d *Deps) HandleCreateProtocolConfig(w http.ResponseWriter, r *http.Request, assetID string) {
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !apiv2.RequireAssetAccess(w, r, assetID) {
		return
	}

	var req protocolConfigRequest
	if err := d.decodeJSONBody(w, r, &req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid request payload")
		return
	}

	req.Protocol = strings.TrimSpace(req.Protocol)
	req.Host = strings.TrimSpace(req.Host)
	req.Username = strings.TrimSpace(req.Username)
	req.CredentialProfileID = strings.TrimSpace(req.CredentialProfileID)

	if err := protocols.ValidateProtocol(req.Protocol); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.Host != "" {
		if err := protocols.ValidateManualDeviceHost(req.Host); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	port := req.Port
	if port == 0 {
		port = protocols.DefaultPort(req.Protocol)
	}
	if err := protocols.ValidatePort(port); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := protocols.ValidateProtocolConfig(req.Protocol, req.Config); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !validateRDPTransportSecurity(w, req.Protocol, req.Config) {
		return
	}
	if !validateVNCTransportSecurity(w, req.Protocol, req.Config, true) {
		return
	}
	if !validateTelnetTransportSecurity(w, req.Protocol, req.Config, true) {
		return
	}

	if !d.authorizeCredentialBinding(w, r, req.CredentialProfileID) {
		return
	}

	existing, err := d.DB.GetProtocolConfig(r.Context(), assetID, req.Protocol)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to check for existing protocol config")
		return
	}
	if existing != nil {
		servicehttp.WriteError(w, http.StatusConflict, "protocol config already exists for this asset")
		return
	}

	pc := protocols.ProtocolConfig{
		ID:                  idgen.New("proto"),
		AssetID:             assetID,
		Protocol:            req.Protocol,
		Host:                req.Host,
		Port:                port,
		Username:            req.Username,
		CredentialProfileID: req.CredentialProfileID,
		Enabled:             true,
		Config:              req.Config,
	}

	if err := d.DB.SaveProtocolConfig(r.Context(), &pc); err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to save protocol config")
		return
	}

	ev := audit.NewEvent("protocol.config.created")
	ev.ActorID = d.principalActorID(r.Context())
	ev.Target = assetID
	ev.Details = map[string]any{
		"protocol": pc.Protocol,
	}
	d.appendAuditEventBestEffort(ev, "api warning: failed to append protocol config create audit event")

	servicehttp.WriteJSON(w, http.StatusCreated, map[string]any{"protocol": pc})
}

// HandleUpdateProtocolConfig handles PUT /assets/{id}/protocols/{protocol}.
func (d *Deps) HandleUpdateProtocolConfig(w http.ResponseWriter, r *http.Request, assetID, protocol string) {
	if r.Method != http.MethodPut {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !apiv2.RequireAssetAccess(w, r, assetID) {
		return
	}

	protocol = strings.TrimSpace(protocol)
	if err := protocols.ValidateProtocol(protocol); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	var req protocolConfigRequest
	if err := d.decodeJSONBody(w, r, &req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid request payload")
		return
	}

	req.Host = strings.TrimSpace(req.Host)
	req.Username = strings.TrimSpace(req.Username)
	req.CredentialProfileID = strings.TrimSpace(req.CredentialProfileID)

	if req.Host != "" {
		if err := protocols.ValidateManualDeviceHost(req.Host); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	port := req.Port
	if port == 0 {
		port = protocols.DefaultPort(protocol)
	}
	if err := protocols.ValidatePort(port); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := protocols.ValidateProtocolConfig(protocol, req.Config); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	if !validateRDPTransportSecurity(w, protocol, req.Config) {
		return
	}
	if !validateVNCTransportSecurity(w, protocol, req.Config, enabled) {
		return
	}
	if !validateTelnetTransportSecurity(w, protocol, req.Config, enabled) {
		return
	}

	if !d.authorizeCredentialBinding(w, r, req.CredentialProfileID) {
		return
	}

	pc := protocols.ProtocolConfig{
		AssetID:             assetID,
		Protocol:            protocol,
		Host:                req.Host,
		Port:                port,
		Username:            req.Username,
		CredentialProfileID: req.CredentialProfileID,
		Enabled:             enabled,
		Config:              req.Config,
	}

	if err := d.DB.SaveProtocolConfig(r.Context(), &pc); err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to save protocol config")
		return
	}

	ev := audit.NewEvent("protocol.config.updated")
	ev.ActorID = d.principalActorID(r.Context())
	ev.Target = assetID
	ev.Details = map[string]any{
		"protocol": protocol,
	}
	d.appendAuditEventBestEffort(ev, "api warning: failed to append protocol config update audit event")

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"protocol": pc})
}

func validateRDPTransportSecurity(w http.ResponseWriter, protocol string, raw json.RawMessage) bool {
	if protocol != protocols.ProtocolRDP {
		return true
	}
	cfg, err := protocols.DecodeRDPConfig(raw)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return false
	}
	if (cfg.IgnoreCertificate || cfg.AllowLegacySecurity) && !securityruntime.InsecureTransportAllowed() {
		servicehttp.WriteError(w, http.StatusBadRequest, "unsafe RDP options require LABTETHER_ALLOW_INSECURE_TRANSPORT=true")
		return false
	}
	return true
}

func validateVNCTransportSecurity(w http.ResponseWriter, protocol string, raw json.RawMessage, enabled bool) bool {
	var allowInsecure bool
	switch protocol {
	case protocols.ProtocolVNC:
		cfg, err := protocols.DecodeVNCConfig(raw)
		if err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
			return false
		}
		allowInsecure = cfg.AllowInsecureTransport
	case protocols.ProtocolARD:
		cfg, err := protocols.DecodeARDConfig(raw)
		if err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
			return false
		}
		allowInsecure = cfg.AllowInsecureTransport
	default:
		return true
	}
	if !enabled {
		return true
	}
	if !allowInsecure {
		servicehttp.WriteError(w, http.StatusBadRequest, "plain VNC requires allow_insecure_vnc=true")
		return false
	}
	if !securityruntime.InsecureTransportAllowed() {
		servicehttp.WriteError(w, http.StatusBadRequest, "plain VNC requires LABTETHER_ALLOW_INSECURE_TRANSPORT=true")
		return false
	}
	return true
}

func validateTelnetTransportSecurity(w http.ResponseWriter, protocol string, raw json.RawMessage, enabled bool) bool {
	if protocol != protocols.ProtocolTelnet {
		return true
	}
	cfg, err := protocols.DecodeTelnetConfig(raw)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return false
	}
	if !enabled {
		return true
	}
	if !cfg.AllowInsecureTransport {
		servicehttp.WriteError(w, http.StatusBadRequest, "plain Telnet requires allow_insecure_telnet=true")
		return false
	}
	if !securityruntime.InsecureTransportAllowed() {
		servicehttp.WriteError(w, http.StatusBadRequest, "plain Telnet requires LABTETHER_ALLOW_INSECURE_TRANSPORT=true")
		return false
	}
	return true
}

func (d *Deps) authorizeCredentialBinding(w http.ResponseWriter, r *http.Request, profileID string) bool {
	profileID = strings.TrimSpace(profileID)
	if profileID == "" {
		return true
	}
	if !apiv2.RequireScope(w, r, "credentials:use") {
		return false
	}
	if d.CredentialStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "credential store unavailable")
		return false
	}
	_, ok, err := d.CredentialStore.GetCredentialProfile(profileID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to validate credential profile")
		return false
	}
	if !ok {
		servicehttp.WriteError(w, http.StatusBadRequest, "credential_profile_id does not reference an existing profile")
		return false
	}
	return true
}

// HandleDeleteProtocolConfig handles DELETE /assets/{id}/protocols/{protocol}.
func (d *Deps) HandleDeleteProtocolConfig(w http.ResponseWriter, r *http.Request, assetID, protocol string) {
	if r.Method != http.MethodDelete {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !apiv2.RequireAssetAccess(w, r, assetID) {
		return
	}

	protocol = strings.TrimSpace(protocol)
	if err := protocols.ValidateProtocol(protocol); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := d.DB.DeleteProtocolConfig(r.Context(), assetID, protocol); err != nil {
		if strings.Contains(err.Error(), "protocol config not found") {
			servicehttp.WriteError(w, http.StatusNotFound, "protocol config not found")
			return
		}
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to delete protocol config")
		return
	}

	ev := audit.NewEvent("protocol.config.deleted")
	ev.ActorID = d.principalActorID(r.Context())
	ev.Target = assetID
	ev.Details = map[string]any{
		"protocol": protocol,
	}
	d.appendAuditEventBestEffort(ev, "api warning: failed to append protocol config delete audit event")

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

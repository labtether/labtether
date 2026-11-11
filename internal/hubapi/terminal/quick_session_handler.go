package terminal

import (
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/policy"
	"github.com/labtether/labtether/internal/servicehttp"
	"github.com/labtether/labtether/internal/terminal"
)

// QuickSessionRequest is the payload for POST /terminal/quick-session.
type QuickSessionRequest struct {
	Host          string `json:"host"`
	Port          int    `json:"port"`
	Username      string `json:"username"`
	AuthMethod    string `json:"auth_method"`
	Password      string `json:"password,omitempty"`    // #nosec G117 -- Session request carries runtime auth material.
	PrivateKey    string `json:"private_key,omitempty"` // #nosec G117 -- Session request carries runtime auth material.
	Passphrase    string `json:"passphrase,omitempty"`
	StrictHostKey bool   `json:"strict_host_key"`
}

// HandleQuickSession creates a terminal session with inline SSH credentials.
// The credentials are stored only in the in-memory session store and never
// written to Postgres.
func (d *Deps) HandleQuickSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if d.TerminalStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "terminal sessions unavailable")
		return
	}
	if !d.EnforceRateLimit(w, r, "terminal.quick_session.create", 60, time.Minute) {
		return
	}

	var req QuickSessionRequest
	if err := d.DecodeJSONBody(w, r, &req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid quick session payload")
		return
	}

	// Normalize and validate fields.
	req.Host = strings.TrimSpace(req.Host)
	req.Username = strings.TrimSpace(req.Username)
	req.AuthMethod = strings.TrimSpace(strings.ToLower(req.AuthMethod))

	if err := ValidateQuickConnectHost(req.Host); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Username == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "username is required")
		return
	}
	if req.Port <= 0 || req.Port > 65535 {
		req.Port = 22
	}

	switch req.AuthMethod {
	case "password":
		if strings.TrimSpace(req.Password) == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "password is required for password auth method")
			return
		}
	case "private_key":
		if strings.TrimSpace(req.PrivateKey) == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "private_key is required for private_key auth method")
			return
		}
	default:
		servicehttp.WriteError(w, http.StatusBadRequest, "auth_method must be password or private_key")
		return
	}

	actorID := d.PrincipalActorID(r.Context())

	// Build the target label for the session (used in logs and UI).
	target := req.Host
	if !d.authorizeTerminalStart(w, r, target) {
		return
	}

	// Connect only to a key the person trusted (POST /terminal/host-keys/trust)
	// or one in the Hub's known_hosts file; the stream checks it strictly.
	strict := effectiveQuickSessionStrictHostKey(req.StrictHostKey)
	hostKey := ""
	if strict {
		pinned, err := d.knownHostKey(req.Host, req.Port)
		if err != nil {
			servicehttp.WriteError(w, http.StatusServiceUnavailable, "LabTether could not read its saved SSH host keys.")
			return
		}
		if needsTrustedHostKey(pinned) {
			servicehttp.WriteJSON(w, http.StatusConflict, map[string]any{
				"error":                          hostKeyUnknownMessage(req.Host, req.Port),
				"requires_host_key_confirmation": true,
			})
			return
		}
		hostKey = pinned
	}

	// Create the session through the standard store.
	session, err := d.TerminalStore.CreateSession(terminal.CreateSessionRequest{
		ActorID: actorID,
		Target:  target,
		Mode:    "interactive",
	})
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to create quick session")
		return
	}

	// Attach inline SSH config and source marker only to the response value. The
	// credential itself is retained in the bounded process-local store below;
	// UpdateSession must never receive it because production uses Postgres.
	session.Source = "quick_connect"
	session.InlineSSHConfig = &terminal.SSHConfig{
		Host:                 req.Host,
		Port:                 req.Port,
		User:                 req.Username,
		Password:             req.Password,
		PrivateKey:           req.PrivateKey,
		PrivateKeyPassphrase: req.Passphrase,
		StrictHostKey:        strict,
		HostKey:              hostKey,
	}
	if d.EphemeralSSHConfigs == nil {
		_ = d.TerminalStore.DeleteTerminalSession(session.ID)
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "quick session credentials unavailable")
		return
	}
	if err := d.EphemeralSSHConfigs.Put(session.ID, session.InlineSSHConfig); err != nil {
		_ = d.TerminalStore.DeleteTerminalSession(session.ID)
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "quick session credential capacity reached")
		return
	}

	// Audit log.
	auditEvent := audit.NewEvent("terminal.quick_session.created")
	auditEvent.ActorID = actorID
	auditEvent.SessionID = session.ID
	auditEvent.Decision = "allowed"
	auditEvent.Details = map[string]any{
		"host":     req.Host,
		"port":     req.Port,
		"username": req.Username,
		"source":   "quick_connect",
	}
	d.AppendAuditEventBestEffort(auditEvent, "api warning: failed to append quick session audit event")

	servicehttp.WriteJSON(w, http.StatusCreated, map[string]any{"session": session})
}

func effectiveQuickSessionStrictHostKey(requested bool) bool {
	return requested || !shared.InsecureSSHHostKeysAllowed()
}

// authorizeTerminalStart applies the checks for starting a terminal session
// on target: access to the target, its maintenance guard and the session_start
// policy. Trusting or removing an SSH host key needs the same.
func (d *Deps) authorizeTerminalStart(w http.ResponseWriter, r *http.Request, target string) bool {
	if !d.requireTargetAccess(w, r, target) {
		return false
	}
	if !d.enforceAssetActionGuard(w, target) {
		return false
	}
	checkRes := policy.Evaluate(policy.CheckRequest{
		ActorID: d.PrincipalActorID(r.Context()),
		Target:  target,
		Mode:    "interactive",
		Action:  "session_start",
	}, d.PolicyState.Current())
	if !checkRes.Allowed {
		servicehttp.WriteError(w, http.StatusForbidden, checkRes.Reason)
		return false
	}
	return true
}

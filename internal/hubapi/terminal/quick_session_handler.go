package terminal

import (
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/audit"
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

	// Policy check: quick sessions must pass session_start just like regular sessions.
	checkRes := policy.Evaluate(policy.CheckRequest{
		ActorID: actorID,
		Target:  target,
		Mode:    "interactive",
		Action:  "session_start",
	}, d.PolicyState.Current())
	if !checkRes.Allowed {
		servicehttp.WriteError(w, http.StatusForbidden, checkRes.Reason)
		return
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

	// Attach inline SSH config and source marker to the in-memory session.
	session.Source = "quick_connect"
	session.InlineSSHConfig = &terminal.SSHConfig{
		Host:                 req.Host,
		Port:                 req.Port,
		User:                 req.Username,
		Password:             req.Password,
		PrivateKey:           req.PrivateKey,
		PrivateKeyPassphrase: req.Passphrase,
		StrictHostKey:        req.StrictHostKey,
	}

	// Persist the updated session (with Source and InlineSSHConfig) back to the store.
	if err := d.TerminalStore.UpdateSession(session); err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to update quick session")
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

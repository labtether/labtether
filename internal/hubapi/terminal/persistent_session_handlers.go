package terminal

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/policy"
	"github.com/labtether/labtether/internal/servicehttp"
	"github.com/labtether/labtether/internal/terminal"
)

func (d *Deps) HandlePersistentSessions(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/terminal/persistent-sessions" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if d.TerminalPersistentStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "persistent terminal sessions are unavailable")
		return
	}

	switch r.Method {
	case http.MethodGet:
		d.listPersistentSessions(w, r)
	case http.MethodPost:
		d.createPersistentSession(w, r)
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (d *Deps) HandlePersistentSessionActions(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/terminal/persistent-sessions/")
	if path == r.URL.Path || path == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "persistent session path not found")
		return
	}
	if d.TerminalPersistentStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "persistent terminal sessions are unavailable")
		return
	}

	parts := strings.Split(path, "/")
	sessionID := strings.TrimSpace(parts[0])
	if sessionID == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "persistent session path not found")
		return
	}

	persistent, ok, err := d.TerminalPersistentStore.GetPersistentSession(sessionID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load persistent session")
		return
	}
	if !ok {
		servicehttp.WriteError(w, http.StatusNotFound, "persistent session not found")
		return
	}
	if !d.canAccessOwnedSession(r, persistent.ActorID) {
		servicehttp.WriteError(w, http.StatusForbidden, "persistent session access denied")
		return
	}

	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"persistent_session": persistent})
		case http.MethodPut:
			d.updatePersistentSession(w, r, persistent)
		case http.MethodDelete:
			d.deletePersistentSession(w, persistent)
		default:
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	if len(parts) == 2 && parts[1] == "attach" {
		if r.Method != http.MethodPost {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		d.attachPersistentSession(w, r, persistent)
		return
	}

	if len(parts) == 2 && parts[1] == "detach" {
		if r.Method != http.MethodPost {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		d.detachPersistentSession(w, persistent)
		return
	}

	servicehttp.WriteError(w, http.StatusNotFound, "unknown persistent session action")
}

func (d *Deps) listPersistentSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := d.TerminalPersistentStore.ListPersistentSessions()
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list persistent sessions")
		return
	}
	actorID := d.PrincipalActorID(r.Context())
	if !d.IsOwnerActor(actorID) {
		filtered := make([]terminal.PersistentSession, 0, len(sessions))
		for _, session := range sessions {
			if strings.TrimSpace(session.ActorID) == actorID {
				filtered = append(filtered, session)
			}
		}
		sessions = filtered
	}
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"persistent_sessions": sessions})
}

func (d *Deps) createPersistentSession(w http.ResponseWriter, r *http.Request) {
	if !d.EnforceRateLimit(w, r, "terminal.persistent_session.create", 60, time.Minute) {
		return
	}

	var req terminal.CreatePersistentSessionRequest
	if err := d.DecodeJSONBody(w, r, &req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid persistent session payload")
		return
	}
	req.ActorID = d.PrincipalActorID(r.Context())
	req.Target = strings.TrimSpace(req.Target)
	req.Title = strings.TrimSpace(req.Title)
	if req.Target == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "target is required")
		return
	}
	if req.Title == "" {
		req.Title = req.Target
	}
	if IsHubLocalTerminalTarget(req.Target) {
		servicehttp.WriteError(w, http.StatusBadRequest, "hub local terminal does not support persistent tmux sessions")
		return
	}
	checkRes := policy.Evaluate(policy.CheckRequest{
		ActorID: req.ActorID,
		Target:  req.Target,
		Mode:    "interactive",
		Action:  "session_start",
	}, d.PolicyState.Current())
	if !checkRes.Allowed {
		servicehttp.WriteError(w, http.StatusForbidden, checkRes.Reason)
		return
	}

	persistent, err := d.TerminalPersistentStore.CreateOrUpdatePersistentSession(req)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to save persistent session")
		return
	}
	servicehttp.WriteJSON(w, http.StatusCreated, map[string]any{"persistent_session": persistent})
}

func (d *Deps) updatePersistentSession(w http.ResponseWriter, r *http.Request, persistent terminal.PersistentSession) {
	if !d.EnforceRateLimit(w, r, "terminal.persistent_session.update", 120, time.Minute) {
		return
	}

	var req terminal.UpdatePersistentSessionRequest
	if err := d.DecodeJSONBody(w, r, &req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid persistent session payload")
		return
	}

	updated, err := d.TerminalPersistentStore.UpdatePersistentSession(persistent.ID, req)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to update persistent session")
		return
	}
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"persistent_session": updated})
}

func (d *Deps) attachPersistentSession(w http.ResponseWriter, r *http.Request, persistent terminal.PersistentSession) {
	checkRes := policy.Evaluate(policy.CheckRequest{
		ActorID: persistent.ActorID,
		Target:  persistent.Target,
		Mode:    "interactive",
		Action:  "session_start",
	}, d.PolicyState.Current())
	if !checkRes.Allowed {
		servicehttp.WriteError(w, http.StatusForbidden, checkRes.Reason)
		return
	}

	attachedAt := time.Now().UTC()
	session, err := d.TerminalStore.CreateSession(terminal.CreateSessionRequest{
		ActorID:             persistent.ActorID,
		Target:              persistent.Target,
		Mode:                "interactive",
		PersistentSessionID: persistent.ID,
		TmuxSessionName:     persistent.TmuxSessionName,
	})
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to create terminal session")
		return
	}

	attached, err := d.TerminalPersistentStore.MarkPersistentSessionAttached(persistent.ID, attachedAt)
	if err != nil {
		_ = d.TerminalStore.DeleteTerminalSession(session.ID)
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to mark persistent session attached")
		return
	}

	ticket, ticketExpiresAt, err := d.IssueStreamTicket(r.Context(), session.ID)
	if err != nil {
		_ = d.TerminalStore.DeleteTerminalSession(session.ID)
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to issue stream ticket")
		return
	}

	servicehttp.WriteJSON(w, http.StatusCreated, map[string]any{
		"persistent_session":      attached,
		"session":                 session,
		"stream_ticket":           ticket,
		"stream_ticket_expires_at": ticketExpiresAt,
	})
}

func (d *Deps) detachPersistentSession(w http.ResponseWriter, persistent terminal.PersistentSession) {
	detached, err := d.TerminalPersistentStore.MarkPersistentSessionDetached(persistent.ID, time.Now().UTC())
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to detach persistent session")
		return
	}
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"persistent_session": detached})
}

func (d *Deps) deletePersistentSession(w http.ResponseWriter, persistent terminal.PersistentSession) {
	if err := d.TerminatePersistentTerminalRuntime(persistent); err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, ErrPersistentTerminalCleanupUnavailable) {
			status = http.StatusConflict
		}
		servicehttp.WriteError(w, status, err.Error())
		return
	}
	sessions, err := d.TerminalStore.ListSessions()
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load attached terminal sessions")
		return
	}
	for _, session := range sessions {
		if strings.TrimSpace(session.PersistentSessionID) == persistent.ID {
			if err := d.TerminalStore.DeleteTerminalSession(session.ID); err != nil {
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to remove attached terminal sessions")
				return
			}
		}
	}
	if err := d.TerminalPersistentStore.DeletePersistentSession(persistent.ID); err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to delete persistent session")
		return
	}
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"deleted": true, "persistent_session_id": persistent.ID})
}

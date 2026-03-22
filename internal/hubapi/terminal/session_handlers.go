package terminal

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/jobqueue"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/policy"
	"github.com/labtether/labtether/internal/servicehttp"
	"github.com/labtether/labtether/internal/terminal"
)

func (d *Deps) HandleSessions(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/terminal/sessions" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if d.TerminalStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "terminal sessions unavailable")
		return
	}

	switch r.Method {
	case http.MethodGet:
		sessions, err := d.TerminalStore.ListSessions()
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list sessions")
			return
		}
		actorID := d.PrincipalActorID(r.Context())
		if !d.IsOwnerActor(actorID) {
			filtered := make([]terminal.Session, 0, len(sessions))
			for _, session := range sessions {
				if strings.TrimSpace(session.ActorID) == actorID {
					filtered = append(filtered, session)
				}
			}
			sessions = filtered
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"sessions": sessions})
	case http.MethodPost:
		d.createSession(w, r)
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (d *Deps) createSession(w http.ResponseWriter, r *http.Request) {
	if !d.EnforceRateLimit(w, r, "terminal.session.create", 60, time.Minute) {
		return
	}

	var req terminal.CreateSessionRequest
	if err := d.DecodeJSONBody(w, r, &req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid session payload")
		return
	}
	requestedActorID := strings.TrimSpace(req.ActorID)
	req.Target = strings.TrimSpace(req.Target)
	req.Mode = strings.TrimSpace(req.Mode)
	if req.Target == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "target is required")
		return
	}
	req.ActorID = d.PrincipalActorID(r.Context())
	if req.Mode == "" {
		req.Mode = "structured"
	}
	if err := d.ValidateMaxLen("actor_id", req.ActorID, d.MaxActorIDLength); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := d.ValidateMaxLen("target", req.Target, d.MaxTargetLength); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := d.ValidateMaxLen("mode", req.Mode, d.MaxModeLength); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	switch strings.ToLower(req.Mode) {
	case "structured", "interactive":
	default:
		servicehttp.WriteError(w, http.StatusBadRequest, "mode must be structured or interactive")
		return
	}

	checkRes := policy.Evaluate(policy.CheckRequest{
		ActorID: req.ActorID,
		Target:  req.Target,
		Mode:    req.Mode,
		Action:  "session_start",
	}, d.PolicyState.Current())
	if !checkRes.Allowed {
		servicehttp.WriteError(w, http.StatusForbidden, checkRes.Reason)
		return
	}

	session, err := d.TerminalStore.CreateSession(req)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to create session")
		return
	}

	auditEvent := audit.NewEvent("terminal.session.created")
	auditEvent.ActorID = session.ActorID
	auditEvent.Target = session.Target
	auditEvent.SessionID = session.ID
	auditEvent.Decision = "allowed"
	auditDetails := map[string]any{"mode": session.Mode}
	if requestedActorID != "" && requestedActorID != session.ActorID {
		auditDetails["requested_actor_label"] = requestedActorID
	}
	auditEvent.Details = auditDetails
	d.AppendAuditEventBestEffort(auditEvent, "api warning: failed to append session audit event")

	servicehttp.WriteJSON(w, http.StatusCreated, map[string]any{"session": session})
}

func (d *Deps) HandleSessionActions(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/terminal/sessions/")
	if path == r.URL.Path || path == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "session path not found")
		return
	}
	if d.TerminalStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "terminal sessions unavailable")
		return
	}

	parts := strings.Split(path, "/")
	sessionID := parts[0]

	session, ok, err := d.TerminalStore.GetSession(sessionID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to read session")
		return
	}
	if !ok {
		servicehttp.WriteError(w, http.StatusNotFound, "session not found")
		return
	}
	if !d.canAccessOwnedSession(r, session.ActorID) {
		servicehttp.WriteError(w, http.StatusForbidden, "session access denied")
		return
	}

	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"session": session})
		case http.MethodDelete:
			if err := d.TerminalStore.DeleteTerminalSession(session.ID); err != nil {
				if errors.Is(err, terminal.ErrSessionNotFound) {
					servicehttp.WriteError(w, http.StatusNotFound, "session not found")
					return
				}
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to delete session")
				return
			}
			servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"deleted": true, "session_id": session.ID})
		default:
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	if len(parts) == 2 && parts[1] == "commands" {
		switch r.Method {
		case http.MethodGet:
			commands, err := d.TerminalStore.ListCommands(sessionID)
			if err != nil {
				if errors.Is(err, terminal.ErrSessionNotFound) {
					servicehttp.WriteError(w, http.StatusNotFound, err.Error())
					return
				}
				servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list commands")
				return
			}
			servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"commands": commands})
		case http.MethodPost:
			d.createCommand(w, r, session)
		default:
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	if len(parts) == 2 && parts[1] == "stream" {
		d.HandleSessionStream(w, r, session)
		return
	}

	if len(parts) == 2 && parts[1] == "stream-ticket" {
		d.HandleSessionStreamTicket(w, r, session)
		return
	}

	if len(parts) == 2 && parts[1] == "scrollback" {
		persistentSessionID := strings.TrimSpace(session.PersistentSessionID)
		if persistentSessionID == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "session is not a persistent session")
			return
		}
		d.handleScrollback(w, r, persistentSessionID)
		return
	}

	servicehttp.WriteError(w, http.StatusNotFound, "unknown session action")
}

func (d *Deps) HandleRecentCommands(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/terminal/commands/recent" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if d.TerminalStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "terminal sessions unavailable")
		return
	}

	limit := shared.ParseLimit(r, 12)
	commands, err := d.TerminalStore.ListRecentCommands(limit)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list recent commands")
		return
	}
	actorID := d.PrincipalActorID(r.Context())
	if !d.IsOwnerActor(actorID) {
		filtered := make([]terminal.Command, 0, len(commands))
		for _, command := range commands {
			if strings.TrimSpace(command.ActorID) == actorID {
				filtered = append(filtered, command)
			}
		}
		commands = filtered
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"commands": commands})
}

func (d *Deps) createCommand(w http.ResponseWriter, r *http.Request, session terminal.Session) {
	if d.TerminalStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "terminal sessions unavailable")
		return
	}
	if !d.EnforceRateLimit(w, r, "terminal.command.create", 180, time.Minute) {
		return
	}

	var req terminal.CreateCommandRequest
	if err := d.DecodeJSONBody(w, r, &req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid command payload")
		return
	}
	req.Command = strings.TrimSpace(req.Command)
	requestedActorID := strings.TrimSpace(req.ActorID)
	if req.Command == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "command is required")
		return
	}
	if err := d.ValidateMaxLen("command", req.Command, d.MaxCommandLength); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	req.ActorID = d.PrincipalActorID(r.Context())
	if err := d.ValidateMaxLen("actor_id", req.ActorID, d.MaxActorIDLength); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	checkRes := policy.Evaluate(policy.CheckRequest{
		ActorID: req.ActorID,
		Target:  session.Target,
		Mode:    session.Mode,
		Action:  "command_execute",
		Command: req.Command,
	}, d.PolicyState.Current())

	auditCheck := audit.NewEvent("terminal.command.policy_checked")
	auditCheck.ActorID = req.ActorID
	auditCheck.Target = session.Target
	auditCheck.SessionID = session.ID
	auditCheck.Decision = "allowed"
	if !checkRes.Allowed {
		auditCheck.Decision = "denied"
		auditCheck.Reason = checkRes.Reason
	}
	auditCheckDetails := map[string]any{"command": req.Command}
	if requestedActorID != "" && requestedActorID != req.ActorID {
		auditCheckDetails["requested_actor_label"] = requestedActorID
	}
	auditCheck.Details = auditCheckDetails
	d.AppendAuditEventBestEffort(auditCheck, "api warning: failed to append policy check audit event")

	if !checkRes.Allowed {
		servicehttp.WriteError(w, http.StatusForbidden, checkRes.Reason)
		return
	}

	cmd, err := d.TerminalStore.AddCommand(session.ID, req, session.Target, session.Mode)
	if err != nil {
		if errors.Is(err, terminal.ErrSessionNotFound) {
			servicehttp.WriteError(w, http.StatusNotFound, err.Error())
			return
		}
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to queue command")
		return
	}

	job := terminal.CommandJob{
		JobID:       idgen.New("job"),
		SessionID:   session.ID,
		CommandID:   cmd.ID,
		ActorID:     req.ActorID,
		Target:      session.Target,
		Command:     req.Command,
		Mode:        session.Mode,
		RequestedAt: cmd.CreatedAt,
	}

	resolvedSSHConfig, err := d.ResolveSessionSSHConfig(session)
	if err != nil {
		_ = d.TerminalStore.UpdateCommandResult(session.ID, cmd.ID, "failed", fmt.Sprintf("ssh config resolution failed: %v", err))
		servicehttp.WriteError(w, http.StatusBadRequest, fmt.Sprintf("unable to resolve ssh config: %v", err))
		return
	}
	job.SSHConfig = resolvedSSHConfig

	if d.JobQueue == nil {
		_ = d.TerminalStore.UpdateCommandResult(session.ID, cmd.ID, "failed", "queue unavailable")
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "command queue unavailable")
		return
	}

	payload, err := json.Marshal(job)
	if err != nil {
		_ = d.TerminalStore.UpdateCommandResult(session.ID, cmd.ID, "failed", "failed to marshal command job")
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to serialize command")
		return
	}

	if _, err := d.JobQueue.Enqueue(r.Context(), jobqueue.KindTerminalCommand, payload); err != nil {
		_ = d.TerminalStore.UpdateCommandResult(session.ID, cmd.ID, "failed", "failed to enqueue command")
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to enqueue command")
		return
	}

	auditQueued := audit.NewEvent("terminal.command.queued")
	auditQueued.ActorID = req.ActorID
	auditQueued.Target = session.Target
	auditQueued.SessionID = session.ID
	auditQueued.CommandID = cmd.ID
	auditQueued.Decision = "queued"
	auditQueuedDetails := map[string]any{
		"job_id":    job.JobID,
		"command":   req.Command,
		"mode":      session.Mode,
		"transport": "postgres",
	}
	if requestedActorID != "" && requestedActorID != req.ActorID {
		auditQueuedDetails["requested_actor_label"] = requestedActorID
	}
	auditQueued.Details = auditQueuedDetails
	d.AppendAuditEventBestEffort(auditQueued, "api warning: failed to append queued audit event")

	d.AppendLogEventBestEffort(logs.Event{
		ID:        fmt.Sprintf("log_command_queued_%s", job.JobID),
		AssetID:   session.Target,
		Source:    "terminal",
		Level:     "info",
		Message:   fmt.Sprintf("command queued: %s", req.Command),
		Timestamp: cmd.CreatedAt,
		Fields: map[string]string{
			"session_id": session.ID,
			"command_id": cmd.ID,
			"job_id":     job.JobID,
		},
	}, "api warning: failed to append queued command log event")

	servicehttp.WriteJSON(w, http.StatusAccepted, map[string]any{
		"job_id":      job.JobID,
		"command":     cmd,
		"queue":       "job_queue",
		"status":      "queued",
		"session":     session.ID,
		"interactive": true,
	})
}

func (d *Deps) canAccessOwnedSession(r *http.Request, sessionActorID string) bool {
	actorID := d.PrincipalActorID(r.Context())
	if d.IsOwnerActor(actorID) {
		return true
	}
	return strings.TrimSpace(sessionActorID) == actorID
}

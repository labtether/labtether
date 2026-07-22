package terminal

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/servicehttp"
	terminalmodel "github.com/labtether/labtether/internal/terminal"
)

const terminalHistoryDeleteLimitPerMinute = 60

// HandleCommandActions deletes one completed command-history record. Session
// ownership and API-key asset restrictions are checked before mutation; active
// commands cannot be deleted because the worker still needs their durable row.
func (d *Deps) HandleCommandActions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if d.TerminalStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "terminal history unavailable")
		return
	}

	commandID := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/terminal/commands/"))
	if commandID == "" || strings.ContainsAny(commandID, "/\x00\r\n") {
		servicehttp.WriteError(w, http.StatusNotFound, "command not found")
		return
	}
	if d.MaxTargetLength > 0 && len([]byte(commandID)) > d.MaxTargetLength {
		servicehttp.WriteError(w, http.StatusNotFound, "command not found")
		return
	}
	if !d.EnforceRateLimit(
		w,
		r,
		"terminal.command.delete",
		terminalHistoryDeleteLimitPerMinute,
		time.Minute,
	) {
		return
	}

	command, ok, err := d.TerminalStore.GetCommand(commandID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to read command history")
		return
	}
	if !ok {
		servicehttp.WriteError(w, http.StatusNotFound, "command not found")
		return
	}

	actorID := d.PrincipalActorID(r.Context())
	if !apiv2.IsOwnerPrincipal(r.Context()) && strings.TrimSpace(command.ActorID) != actorID {
		servicehttp.WriteError(w, http.StatusForbidden, "command access denied")
		return
	}
	if !d.requireTargetAccess(w, r, command.Target) {
		return
	}
	if !terminalCommandHistoryDeletable(command.Status) {
		servicehttp.WriteError(w, http.StatusConflict, "command is still active")
		return
	}

	if err := d.TerminalStore.DeleteCommand(command.ID); err != nil {
		if errors.Is(err, terminalmodel.ErrCommandNotFound) {
			servicehttp.WriteError(w, http.StatusNotFound, "command not found")
			return
		}
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to delete command history")
		return
	}

	auditEvent := audit.NewEvent("terminal.command.deleted")
	auditEvent.ActorID = actorID
	auditEvent.Target = command.Target
	auditEvent.SessionID = command.SessionID
	auditEvent.CommandID = command.ID
	auditEvent.Decision = "allowed"
	auditEvent.Details = map[string]any{"previous_status": command.Status}
	d.AppendAuditEventBestEffort(auditEvent, "api warning: failed to append command-history deletion audit event")

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"deleted":    true,
		"command_id": command.ID,
	})
}

func terminalCommandHistoryDeletable(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "succeeded", "completed", "success", "failed", "error", "cancelled", "canceled", "timed_out", "timeout":
		return true
	default:
		return false
	}
}

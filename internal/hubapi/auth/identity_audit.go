package auth

import (
	"net/http"
	"strings"

	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/auth"
)

const identityAuditWarning = "api warning: failed to append identity change audit event"

// auditIdentityChange records who changed an account, role, password, 2FA,
// sessions or sign-in settings. The generic request event only knows the
// client address, which behind the bundled console is the console itself.
func (d *Deps) auditIdentityChange(r *http.Request, eventType, targetUserID string, details map[string]any) {
	if d == nil || d.AppendAuditEventBestEffort == nil || r == nil {
		return
	}
	event := audit.NewEvent(eventType)
	event.ActorID = d.actorIDFromContext(r.Context())
	event.Target = strings.TrimSpace(targetUserID)
	event.Decision = "applied"
	event.Details = details
	d.AppendAuditEventBestEffort(event, identityAuditWarning)
}

// callerMayManageUser reports whether the caller may change, sign out or
// delete target. Only the owner may act on another admin or on the owner, so
// one admin cannot take over another admin or keep signing the owner out.
func (d *Deps) callerMayManageUser(r *http.Request, target auth.User) bool {
	if d.actorIDFromContext(r.Context()) == target.ID {
		return true
	}
	if !auth.HasAdminPrivileges(target.Role) {
		return true
	}
	return d.callerIsOwner(r)
}

// callerMayGrantRole reports whether the caller may give role to an account
// that does not hold it yet. Only the owner may make an admin; admins manage
// operators and viewers.
func (d *Deps) callerMayGrantRole(r *http.Request, role string) bool {
	return !auth.HasAdminPrivileges(role) || d.callerIsOwner(r)
}

func (d *Deps) callerIsOwner(r *http.Request) bool {
	return d.UserRoleFromContext != nil && auth.NormalizeRole(d.UserRoleFromContext(r.Context())) == auth.RoleOwner
}

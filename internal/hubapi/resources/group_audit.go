package resources

import (
	"net/http"

	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/groups"
)

const (
	groupCreatedAuditType = "group.created"
	groupUpdatedAuditType = "group.updated"
	groupDeletedAuditType = "group.deleted"
)

// auditGroup records who created, changed or deleted a group, with the
// group's name so the row still reads after the group is gone. Dragging a
// group to a new position (sort order only) is not recorded.
func (d *Deps) auditGroup(r *http.Request, eventType string, group groups.Group, details map[string]any) {
	if d.AppendAuditEventBestEffort == nil {
		return
	}
	event := audit.NewEvent(eventType)
	if d.PrincipalActorID != nil {
		event.ActorID = d.PrincipalActorID(r.Context())
	}
	event.Target = group.ID
	event.TargetName = group.Name
	event.Decision = "applied"
	event.Details = details
	d.AppendAuditEventBestEffort(event, "api warning: failed to append group audit event")
}

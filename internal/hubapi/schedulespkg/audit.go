package schedulespkg

import (
	"net/http"
	"time"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/schedules"
)

const (
	scheduleCreatedAuditType = "schedule.created"
	scheduleUpdatedAuditType = "schedule.updated"
	scheduleDeletedAuditType = "schedule.deleted"
)

// auditSchedule records who created, changed or deleted a schedule, with the
// schedule's name so the row still reads after the schedule is gone. The
// command never goes into the row: scheduled commands can carry secrets.
func (d *Deps) auditSchedule(r *http.Request, eventType string, task schedules.ScheduledTask, details map[string]any) {
	shared.AppendAuditEventBestEffort(d.AuditStore, audit.Event{
		Type:       eventType,
		ActorID:    apiv2.PrincipalActorID(r.Context()),
		Target:     task.ID,
		TargetName: task.Name,
		Decision:   "applied",
		Details:    details,
		Timestamp:  time.Now().UTC(),
	}, "api warning: failed to append schedule audit event")
}

package groupfeatures

import (
	"net/http"
	"time"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/groupmaintenance"
	"github.com/labtether/labtether/internal/hubapi/shared"
)

const (
	maintenanceWindowCreatedAuditType = "maintenance_window.created"
	maintenanceWindowUpdatedAuditType = "maintenance_window.updated"
	maintenanceWindowDeletedAuditType = "maintenance_window.deleted"
)

// auditMaintenanceWindow records who created, changed or deleted a maintenance
// window, with the window's and its group's names so the row still reads after
// either is gone.
func (d *Deps) auditMaintenanceWindow(r *http.Request, eventType string, window groupmaintenance.MaintenanceWindow) {
	details := map[string]any{
		"group_id":        window.GroupID,
		"suppress_alerts": window.SuppressAlerts,
		"block_actions":   window.BlockActions,
		"block_updates":   window.BlockUpdates,
	}
	if d.GroupStore != nil {
		if group, ok, err := d.GroupStore.GetGroup(window.GroupID); err == nil && ok {
			details["group_name"] = group.Name
		}
	}
	if !window.EndAt.IsZero() {
		details["start_at"] = window.StartAt.UTC().Format(time.RFC3339)
		details["end_at"] = window.EndAt.UTC().Format(time.RFC3339)
	}
	event := audit.NewEvent(eventType)
	event.ActorID = apiv2.PrincipalActorID(r.Context())
	event.Target = window.ID
	event.TargetName = window.Name
	event.Decision = "applied"
	event.Details = details
	shared.AppendAuditEventBestEffort(d.AuditStore, event, "api warning: failed to append maintenance window audit event")
}

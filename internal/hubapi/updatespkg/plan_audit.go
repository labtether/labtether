package updatespkg

import (
	"net/http"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/updates"
)

const (
	updatePlanCreatedAuditType = "updates.plan.created"
	updatePlanDeletedAuditType = "updates.plan.deleted"
)

// auditUpdatePlan records who created or deleted an update plan, with the
// plan's name so the row still reads after the plan is gone.
func (d *Deps) auditUpdatePlan(r *http.Request, eventType string, plan updates.Plan) {
	if d.AppendAuditEventBestEffort == nil {
		return
	}
	event := audit.NewEvent(eventType)
	event.ActorID = apiv2.PrincipalActorID(r.Context())
	event.Target = plan.ID
	event.TargetName = plan.Name
	event.Decision = "applied"
	event.Details = map[string]any{
		"targets":         len(plan.Targets),
		"scopes":          plan.Scopes,
		"default_dry_run": plan.DefaultDryRun,
	}
	d.AppendAuditEventBestEffort(event, "api warning: failed to append update plan audit event")
}

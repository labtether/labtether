package alerting

import (
	"context"
	"fmt"
	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/incidents"
	"github.com/labtether/labtether/internal/persistence"
	"log"
	"time"
)

func (d *Deps) MaybeAutoCreateIncident(rule alerts.Rule, instance alerts.AlertInstance) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	d.maybeAutoCreateIncidentContext(ctx, rule, instance)
}

func (d *Deps) maybeAutoCreateIncidentContext(ctx context.Context, rule alerts.Rule, instance alerts.AlertInstance) {
	if d.IncidentStore == nil || ctx.Err() != nil {
		return
	}

	// Only auto-create for critical severity
	if rule.Severity != alerts.SeverityCritical {
		return
	}

	// Only if firing for > 5 minutes
	if time.Since(instance.StartedAt) < 5*time.Minute {
		return
	}

	store, ok := d.IncidentStore.(persistence.AlertAutoIncidentContextStore)
	if !ok {
		log.Printf("alert evaluator: context-aware auto incident store unavailable")
		return
	}
	// The store owns duplicate prevention and both writes in one operation.
	inc, created, err := store.CreateAutoIncidentForAlertInstanceContext(ctx, instance.ID, incidents.CreateIncidentRequest{
		Title:    fmt.Sprintf("Auto: %s", rule.Name),
		Severity: incidents.SeverityCritical,
		Source:   incidents.SourceAlertAuto,
		Summary:  fmt.Sprintf("Auto-created from critical alert rule %q firing for >5 minutes", rule.Name),
	})
	if err != nil {
		log.Printf("alert evaluator: failed to auto-create incident: %v", err)
		return
	}

	if !created || ctx.Err() != nil {
		return
	}
	d.dispatchIncidentNotificationAsync(inc, "incident.created")
	log.Printf("alert evaluator: auto-created incident %s from critical alert %s", inc.ID, rule.Name)
}

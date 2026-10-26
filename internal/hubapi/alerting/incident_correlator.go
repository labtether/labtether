package alerting

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/incidents"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/updates"
)

// correlationPage holds bounded candidates for the shared scope/window matcher.
type correlationPage struct {
	actionRuns     []actions.Run
	updateRuns     []updates.Run
	alertInstances []alerts.AlertInstance
	auditEvents    []audit.Event
}

func (d *Deps) RunIncidentCorrelator(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	log.Printf("incident correlator started (interval=30s)")

	for {
		select {
		case <-ctx.Done():
			log.Printf("incident correlator stopped")
			return
		case <-ticker.C:
			d.correlateOpenIncidents(ctx)
		}
	}
}

func (d *Deps) correlateIncidentPage(ctx context.Context, inc incidents.Incident, scope incidentCorrelationScope, pf correlationPage, window persistence.CorrelationWindow) error {
	from, now := window.From, window.Through

	// Check each candidate against the same scope and fixed event-time window.
	// Correlate failed action runs.
	for _, run := range pf.actionRuns {
		if run.Status != actions.StatusFailed || !scope.matchesOperation(run.Target) {
			continue
		}
		if run.CreatedAt.Before(from) {
			continue
		}
		if run.CreatedAt.After(now) {
			continue
		}
		if err := d.upsertCorrelationEvent(ctx, inc.ID, incidents.EventTypeActionRun,
			fmt.Sprintf("action_runs:%s", run.ID),
			fmt.Sprintf("Failed action run: %s", run.Type),
			"warning",
			run.CreatedAt,
		); err != nil {
			return err
		}
	}

	// Correlate failed update runs.
	for _, run := range pf.updateRuns {
		if run.Status != updates.StatusFailed || !scope.matchesUpdate(run) {
			continue
		}
		if run.CreatedAt.Before(from) {
			continue
		}
		if run.CreatedAt.After(now) {
			continue
		}
		if err := d.upsertCorrelationEvent(ctx, inc.ID, incidents.EventTypeUpdateRun,
			fmt.Sprintf("update_runs:%s", run.ID),
			fmt.Sprintf("Failed update run for plan %s", run.PlanID),
			"warning",
			run.CreatedAt,
		); err != nil {
			return err
		}
	}

	// Correlate alert instances fired during window.
	for _, inst := range pf.alertInstances {
		if !scope.matchesAlert(inst) {
			continue
		}
		if inst.FirstFiredAt.Before(from) {
			continue
		}
		if inst.FirstFiredAt.After(now) {
			continue
		}
		if err := d.upsertCorrelationEvent(ctx, inc.ID, incidents.EventTypeAlertFired,
			fmt.Sprintf("alert_instances:%s", inst.ID),
			fmt.Sprintf("Alert fired: rule %s (severity: %s)", inst.RuleID, inst.Severity),
			inst.Severity,
			*inst.FirstFiredAt,
		); err != nil {
			return err
		}
	}

	// Correlate audit events.
	// Ordering never replaces the exact event-time and scope checks.
	for _, evt := range pf.auditEvents {
		if evt.Target != inc.ID && !scope.matchesOperation(evt.Target) {
			continue
		}
		if evt.Timestamp.Before(from) || evt.Timestamp.After(now) {
			continue
		}
		if err := d.upsertCorrelationEvent(ctx, inc.ID, incidents.EventTypeAudit,
			fmt.Sprintf("audit_events:%s", evt.ID),
			fmt.Sprintf("Audit: %s by %s", evt.Type, evt.ActorID),
			"info",
			evt.Timestamp,
		); err != nil {
			return err
		}
	}
	return ctx.Err()
}

func (d *Deps) upsertCorrelationEvent(ctx context.Context, incidentID, eventType, sourceRef, summary, severity string, occurredAt time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	owner, ok := d.IncidentEventStore.(persistence.IncidentEventContextStore)
	if !ok {
		return fmt.Errorf("incident event context owner unavailable")
	}
	_, err := owner.UpsertIncidentEventContext(ctx, incidents.CreateIncidentEventRequest{
		IncidentID: incidentID, EventType: eventType, SourceRef: sourceRef,
		Summary: summary, Severity: severity, OccurredAt: occurredAt,
	})
	return err
}

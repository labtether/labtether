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

// correlationPrefetch holds data fetched once per correlator cycle so that
// correlateIncident does not issue N independent queries per incident.
type correlationPrefetch struct {
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

func (d *Deps) correlateOpenIncidents(ctx context.Context) {
	if d.IncidentStore == nil || d.IncidentEventStore == nil {
		return
	}

	openIncidents, err := d.IncidentStore.ListIncidents(persistence.IncidentFilter{
		Limit:  100,
		Status: incidents.StatusOpen,
	})
	if err != nil {
		log.Printf("incident correlator: failed to list open incidents: %v", err)
		return
	}

	investigating, err := d.IncidentStore.ListIncidents(persistence.IncidentFilter{
		Limit:  100,
		Status: incidents.StatusInvestigating,
	})
	if err == nil {
		openIncidents = append(openIncidents, investigating...)
	}

	if len(openIncidents) == 0 {
		return
	}

	// Prefetch all correlation datasets once for this cycle.
	prefetch := d.prefetchCorrelationData()

	for _, inc := range openIncidents {
		select {
		case <-ctx.Done():
			return
		default:
		}
		d.correlateIncident(inc, prefetch)
	}
}

// prefetchCorrelationData fetches all datasets needed for incident correlation
// in a single pass so that correlateIncident can work from in-memory slices.
func (d *Deps) prefetchCorrelationData() correlationPrefetch {
	var pf correlationPrefetch

	if d.ActionStore != nil {
		runs, err := d.ActionStore.ListActionRuns(100, 0, "", "failed")
		if err == nil {
			pf.actionRuns = runs
		}
	}

	if d.UpdateStore != nil {
		runs, err := d.UpdateStore.ListUpdateRuns(100, "failed")
		if err == nil {
			pf.updateRuns = runs
		}
	}

	if d.AlertInstanceStore != nil {
		instances, err := d.AlertInstanceStore.ListAlertInstances(persistence.AlertInstanceFilter{
			Limit: 100,
		})
		if err == nil {
			pf.alertInstances = instances
		}
	}

	if d.AuditStore != nil {
		events, err := d.AuditStore.List(100, 0)
		if err == nil {
			pf.auditEvents = events
		}
	}

	return pf
}

func (d *Deps) correlateIncident(inc incidents.Incident, pf correlationPrefetch) {
	// Determine time window: from incident opened_at to now
	from := inc.OpenedAt
	now := time.Now().UTC()

	// Correlate failed action runs.
	// Results are ordered by updated_at DESC, so the first record whose
	// CreatedAt falls before the incident window means all remaining records
	// are also outside the window — we can stop iterating early.
	for _, run := range pf.actionRuns {
		if run.CreatedAt.Before(from) {
			break // all subsequent records are older
		}
		if run.CreatedAt.After(now) {
			continue
		}
		d.upsertCorrelationEvent(inc.ID, incidents.EventTypeActionRun,
			fmt.Sprintf("action_runs:%s", run.ID),
			fmt.Sprintf("Failed action run: %s", run.Type),
			"warning",
			run.CreatedAt,
		)
	}

	// Correlate failed update runs.
	// Results are ordered by updated_at DESC; same early-break applies.
	for _, run := range pf.updateRuns {
		if run.CreatedAt.Before(from) {
			break // all subsequent records are older
		}
		if run.CreatedAt.After(now) {
			continue
		}
		d.upsertCorrelationEvent(inc.ID, incidents.EventTypeUpdateRun,
			fmt.Sprintf("update_runs:%s", run.ID),
			fmt.Sprintf("Failed update run for plan %s", run.PlanID),
			"warning",
			run.CreatedAt,
		)
	}

	// Correlate alert instances fired during window.
	// Results are ordered by updated_at DESC; break once StartedAt is before
	// the window since older instances cannot match.
	for _, inst := range pf.alertInstances {
		if inst.StartedAt.Before(from) {
			break // all subsequent instances are older
		}
		if inst.StartedAt.After(now) {
			continue
		}
		d.upsertCorrelationEvent(inc.ID, incidents.EventTypeAlertFired,
			fmt.Sprintf("alert_instances:%s", inst.ID),
			fmt.Sprintf("Alert fired: rule %s (severity: %s)", inst.RuleID, inst.Severity),
			inst.Severity,
			inst.StartedAt,
		)
	}

	// Correlate audit events.
	// Results are in chronological (oldest-first) order, so we must scan all.
	for _, evt := range pf.auditEvents {
		if evt.Timestamp.Before(from) || evt.Timestamp.After(now) {
			continue
		}
		d.upsertCorrelationEvent(inc.ID, incidents.EventTypeAudit,
			fmt.Sprintf("audit_events:%s", evt.ID),
			fmt.Sprintf("Audit: %s by %s", evt.Type, evt.ActorID),
			"info",
			evt.Timestamp,
		)
	}
}

func (d *Deps) upsertCorrelationEvent(incidentID, eventType, sourceRef, summary, severity string, occurredAt time.Time) {
	_, err := d.IncidentEventStore.UpsertIncidentEvent(incidents.CreateIncidentEventRequest{
		IncidentID: incidentID,
		EventType:  eventType,
		SourceRef:  sourceRef,
		Summary:    summary,
		Severity:   severity,
		OccurredAt: occurredAt,
	})
	if err != nil {
		log.Printf("incident correlator: failed to upsert event for incident %s: %v", incidentID, err)
	}
}

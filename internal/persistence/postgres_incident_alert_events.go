package persistence

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/incidents"
)

// incidentAlert is what an issue's timeline needs about one alert instance:
// its rule, the rule's name and the one device it fired for (when it fired
// for exactly one).
type incidentAlert struct {
	instanceID string
	ruleID     string
	ruleName   string
	severity   string
	deviceID   string
	deviceName string
	firedAt    time.Time
	resolvedAt *time.Time
}

// singleFiringAssetSQL is the one asset ID in an instance's firing scope, or
// NULL. Nested CASE keeps jsonb_array_length away from a non-array value.
func singleFiringAssetSQL(alias string) string {
	scope := alias + ".firing_scope->'asset_ids'"
	return "CASE WHEN jsonb_typeof(" + scope + ") = 'array' THEN CASE WHEN jsonb_array_length(" + scope +
		") = 1 THEN " + scope + "->>0 END END"
}

var incidentAlertSQL = `SELECT ai.id, ai.rule_id, COALESCE(r.name, ''), ai.severity,
	COALESCE(a.id, ''), COALESCE(a.name, ''), COALESCE(ai.first_fired_at, ai.started_at), ai.resolved_at
 FROM alert_instances ai
 LEFT JOIN alert_rules r ON r.id = ai.rule_id
 LEFT JOIN assets a ON a.id = (` + singleFiringAssetSQL("ai") + `)
 WHERE ai.id = $1`

// readIncidentAlert loads the instance. A missing instance is not an error:
// the caller then works from the instance ID alone.
func readIncidentAlert(ctx context.Context, tx pgx.Tx, instanceID string) (incidentAlert, bool, error) {
	var alert incidentAlert
	err := tx.QueryRow(ctx, incidentAlertSQL, instanceID).Scan(&alert.instanceID, &alert.ruleID, &alert.ruleName,
		&alert.severity, &alert.deviceID, &alert.deviceName, &alert.firedAt, &alert.resolvedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return incidentAlert{}, false, nil
	}
	if err != nil {
		return incidentAlert{}, false, err
	}
	alert.firedAt = alert.firedAt.UTC()
	return alert, true, nil
}

// alertEventSummary is the timeline line: "High CPU fired on nas01".
func alertEventSummary(alert incidentAlert, verb string) string {
	name := strings.TrimSpace(alert.ruleName)
	if name == "" {
		name = "An alert"
	}
	if device := strings.TrimSpace(alert.deviceName); device != "" {
		return fmt.Sprintf("%s %s on %s", name, verb, device)
	}
	return name + " " + verb
}

// upsertAlertIncidentEvent writes one timeline line inside tx. The source
// reference matches the incident correlator's, so neither writes it twice.
func upsertAlertIncidentEvent(ctx context.Context, tx pgx.Tx, incidentID, eventType, sourceRef, summary string, alert incidentAlert, at time.Time) error {
	metadata, err := marshalAnyMap(map[string]any{"alert_rule_id": alert.ruleID, "alert_instance_id": alert.instanceID})
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO incident_events
		(id, incident_id, event_type, source_ref, summary, severity, metadata, occurred_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, $9)
		ON CONFLICT (incident_id, source_ref) DO UPDATE
		SET summary = EXCLUDED.summary, severity = EXCLUDED.severity,
		    metadata = EXCLUDED.metadata, occurred_at = EXCLUDED.occurred_at`,
		idgen.New("ievt"), incidentID, eventType, sourceRef, summary, nullIfBlank(alert.severity),
		metadata, at.UTC(), time.Now().UTC())
	return err
}

// recordAlertFired puts the instance's firing on the issue's timeline.
func recordAlertFired(ctx context.Context, tx pgx.Tx, incidentID string, alert incidentAlert) error {
	return upsertAlertIncidentEvent(ctx, tx, incidentID, incidents.EventTypeAlertFired,
		"alert_instances:"+alert.instanceID, alertEventSummary(alert, "fired"), alert, alert.firedAt)
}

// recordAlertResolvedOnIncidents adds "<rule> recovered" to every issue that
// links the instance and is not closed. It runs in its own savepoint: a
// timeline write that fails must never stop the alert from resolving.
func recordAlertResolvedOnIncidents(ctx context.Context, tx pgx.Tx, instanceID string) {
	if err := recordAlertResolvedInSavepoint(ctx, tx, instanceID); err != nil {
		log.Printf("alert instances: failed to add recovery of %s to its issues: %v", instanceID, err)
	}
}

func recordAlertResolvedInSavepoint(ctx context.Context, tx pgx.Tx, instanceID string) error {
	savepoint, err := tx.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = savepoint.Rollback(ctx) }()
	alert, ok, err := readIncidentAlert(ctx, savepoint, instanceID)
	if err != nil || !ok || alert.resolvedAt == nil {
		return err
	}
	rows, err := savepoint.Query(ctx, `SELECT DISTINCT links.incident_id FROM incident_alert_links links
		JOIN incidents ON incidents.id = links.incident_id
		WHERE links.alert_instance_id = $1 AND incidents.status <> $2`, instanceID, incidents.StatusClosed)
	if err != nil {
		return err
	}
	incidentIDs, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return err
	}
	for _, incidentID := range incidentIDs {
		if err := upsertAlertIncidentEvent(ctx, savepoint, incidentID, incidents.EventTypeAlertResolved,
			"alert_instances:"+instanceID+":resolved", alertEventSummary(alert, "recovered"), alert, *alert.resolvedAt); err != nil {
			return err
		}
	}
	return savepoint.Commit(ctx)
}

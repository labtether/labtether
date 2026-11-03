package persistence

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/incidents"
)

// CreateAutoIncidentForAlertInstanceContext opens one issue per firing rule.
// When the rule already has an automatic issue that is still open
// (open, investigating or mitigated), the new instance is linked to it and
// its firing goes on that issue's timeline instead of opening a second issue.
// A new issue starts with the firing alert on its timeline, and names the
// device as its primary asset when the alert fired for exactly one device.
func (s *PostgresStore) CreateAutoIncidentForAlertInstanceContext(ctx context.Context, instanceID string, req incidents.CreateIncidentRequest) (incidents.Incident, bool, error) {
	instanceID = strings.TrimSpace(instanceID)
	if instanceID == "" {
		return incidents.Incident{}, false, incidents.ErrAlertReferenceRequired
	}
	if err := ctx.Err(); err != nil {
		return incidents.Incident{}, false, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return incidents.Incident{}, false, err
	}
	defer func() {
		// A canceled caller still needs bounded transaction cleanup.
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	// The instance's rule never changes, so it can be read before the lock.
	alert, known, err := readIncidentAlert(ctx, tx, instanceID)
	if err != nil {
		return incidents.Incident{}, false, err
	}
	// All automatic creators for one rule serialize before the lookups.
	// READ COMMITTED sees the preceding creator's committed rows after waiting.
	lockKey := "labtether:auto-incident:" + instanceID
	if known {
		lockKey = "labtether:auto-incident-rule:" + alert.ruleID
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, lockKey); err != nil {
		return incidents.Incident{}, false, err
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM incident_alert_links links
		JOIN incidents ON incidents.id = links.incident_id
		WHERE links.alert_instance_id = $1 AND incidents.source = $2
	)`, instanceID, incidents.SourceAlertAuto).Scan(&exists); err != nil {
		return incidents.Incident{}, false, err
	}
	if exists {
		return incidents.Incident{}, false, nil
	}
	if known {
		linked, err := linkToOpenAutoIncident(ctx, tx, alert)
		if err != nil {
			return incidents.Incident{}, false, err
		}
		if linked {
			if err := tx.Commit(ctx); err != nil {
				return incidents.Incident{}, false, err
			}
			return incidents.Incident{}, false, nil
		}
	}
	req.Source = incidents.SourceAlertAuto
	if known && alert.deviceID != "" && strings.TrimSpace(req.PrimaryAssetID) == "" {
		req.PrimaryAssetID = alert.deviceID
	}
	incident, err := createIncidentContext(ctx, tx, req)
	if err != nil {
		return incidents.Incident{}, false, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO incident_alert_links
		(id, incident_id, alert_instance_id, link_type, created_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`, idgen.New("inclink"), incident.ID,
		instanceID, incidents.LinkTypeTrigger, incident.CreatedBy, incident.CreatedAt); err != nil {
		return incidents.Incident{}, false, err
	}
	if known {
		if err := recordAlertFired(ctx, tx, incident.ID, alert); err != nil {
			return incidents.Incident{}, false, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return incidents.Incident{}, false, err
	}
	return incident, true, nil
}

// linkToOpenAutoIncident attaches the instance to the newest automatic issue
// of the same rule that is still open, and reports whether there was one.
func linkToOpenAutoIncident(ctx context.Context, tx pgx.Tx, alert incidentAlert) (bool, error) {
	var incidentID, createdBy string
	err := tx.QueryRow(ctx, `SELECT incidents.id, incidents.created_by FROM incidents
		JOIN incident_alert_links links ON links.incident_id = incidents.id
		JOIN alert_instances ON alert_instances.id = links.alert_instance_id
		WHERE incidents.source = $1 AND alert_instances.rule_id = $2
		  AND incidents.status IN ($3, $4, $5)
		ORDER BY incidents.opened_at DESC, incidents.id DESC
		LIMIT 1`, incidents.SourceAlertAuto, alert.ruleID,
		incidents.StatusOpen, incidents.StatusInvestigating, incidents.StatusMitigated).Scan(&incidentID, &createdBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO incident_alert_links
		(id, incident_id, alert_instance_id, link_type, created_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`, idgen.New("inclink"), incidentID,
		alert.instanceID, incidents.LinkTypeRelated, createdBy, time.Now().UTC()); err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `UPDATE incidents SET updated_at = $2 WHERE id = $1`, incidentID, time.Now().UTC()); err != nil {
		return false, err
	}
	return true, recordAlertFired(ctx, tx, incidentID, alert)
}

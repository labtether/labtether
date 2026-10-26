package persistence

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/incidents"
)

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
	// All automatic creators for this instance serialize before the lookup.
	// READ COMMITTED sees the preceding creator's committed pair after waiting.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, "labtether:auto-incident:"+instanceID); err != nil {
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
	req.Source = incidents.SourceAlertAuto
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
	if err := tx.Commit(ctx); err != nil {
		return incidents.Incident{}, false, err
	}
	return incident, true, nil
}

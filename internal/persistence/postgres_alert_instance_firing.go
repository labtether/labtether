package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/alerts"
)

func (s *PostgresStore) FireAlertInstance(ctx context.Context, id string, scope alerts.FiringScope) (alerts.AlertInstance, error) {
	normalized, err := alerts.NormalizeFiringScope(scope)
	if err != nil {
		return alerts.AlertInstance{}, err
	}
	payload, err := json.Marshal(normalized)
	if err != nil {
		return alerts.AlertInstance{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return alerts.AlertInstance{}, err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	inst, err := scanAlertInstance(tx.QueryRow(ctx, `SELECT id, rule_id, fingerprint, status, severity,
  labels, annotations, started_at, resolved_at, last_fired_at,
  suppressed_by, created_at, updated_at, first_fired_at, firing_scope
  FROM alert_instances WHERE id=$1 FOR UPDATE`, strings.TrimSpace(id)))
	if err != nil {
		return alerts.AlertInstance{}, err
	}
	if inst.Status == alerts.InstanceStatusFiring || inst.Status == alerts.InstanceStatusAcknowledged {
		return inst, nil
	}
	if inst.Status != alerts.InstanceStatusPending || inst.FirstFiredAt != nil || inst.FiringScope != nil {
		return alerts.AlertInstance{}, errors.New("alert instance is not a fresh pending instance")
	}
	updated, err := scanAlertInstance(tx.QueryRow(ctx, `UPDATE alert_instances
  SET status='firing', first_fired_at=$2, last_fired_at=$2, updated_at=$2, firing_scope=$3::jsonb
  WHERE id=$1 RETURNING id, rule_id, fingerprint, status, severity,
  labels, annotations, started_at, resolved_at, last_fired_at,
  suppressed_by, created_at, updated_at, first_fired_at, firing_scope`, inst.ID, time.Now().UTC(), payload))
	if err != nil {
		return alerts.AlertInstance{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return alerts.AlertInstance{}, err
	}
	return updated, nil
}

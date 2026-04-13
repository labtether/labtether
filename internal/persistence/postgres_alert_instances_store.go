package persistence

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/idgen"
)

func (s *PostgresStore) CreateAlertInstance(req alerts.CreateInstanceRequest) (alerts.AlertInstance, error) {
	now := time.Now().UTC()
	severity := alerts.NormalizeSeverity(req.Severity)
	if severity == "" {
		severity = alerts.SeverityMedium
	}
	fingerprint := strings.TrimSpace(req.Fingerprint)
	if fingerprint == "" {
		fingerprint = alerts.GenerateFingerprint(req.RuleID, req.Labels)
	}

	labelsPayload, err := marshalStringMap(req.Labels)
	if err != nil {
		return alerts.AlertInstance{}, err
	}
	annotationsPayload, err := marshalStringMap(req.Annotations)
	if err != nil {
		return alerts.AlertInstance{}, err
	}

	return scanAlertInstance(s.pool.QueryRow(context.Background(),
		`INSERT INTO alert_instances (
			id, rule_id, fingerprint, status, severity,
			labels, annotations, started_at, last_fired_at,
			created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7::jsonb, $8, $8, $8, $8)
		RETURNING
			id, rule_id, fingerprint, status, severity,
			labels, annotations, started_at, resolved_at, last_fired_at,
			suppressed_by, created_at, updated_at`,
		idgen.New("ainst"),
		strings.TrimSpace(req.RuleID),
		fingerprint,
		alerts.InstanceStatusPending,
		severity,
		labelsPayload,
		annotationsPayload,
		now,
	))
}

func (s *PostgresStore) GetAlertInstance(id string) (alerts.AlertInstance, bool, error) {
	inst, err := scanAlertInstance(s.pool.QueryRow(context.Background(),
		`SELECT id, rule_id, fingerprint, status, severity,
			labels, annotations, started_at, resolved_at, last_fired_at,
			suppressed_by, created_at, updated_at
		 FROM alert_instances WHERE id = $1`,
		strings.TrimSpace(id),
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return alerts.AlertInstance{}, false, nil
		}
		return alerts.AlertInstance{}, false, err
	}
	return inst, true, nil
}

func (s *PostgresStore) GetActiveInstanceByFingerprint(ruleID, fingerprint string) (alerts.AlertInstance, bool, error) {
	inst, err := scanAlertInstance(s.pool.QueryRow(context.Background(),
		`SELECT id, rule_id, fingerprint, status, severity,
			labels, annotations, started_at, resolved_at, last_fired_at,
			suppressed_by, created_at, updated_at
		 FROM alert_instances
		 WHERE rule_id = $1 AND fingerprint = $2 AND status IN ('pending', 'firing', 'acknowledged')`,
		strings.TrimSpace(ruleID),
		strings.TrimSpace(fingerprint),
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return alerts.AlertInstance{}, false, nil
		}
		return alerts.AlertInstance{}, false, err
	}
	return inst, true, nil
}

func (s *PostgresStore) ListAlertInstances(filter AlertInstanceFilter) ([]alerts.AlertInstance, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	where := make([]string, 0, 3)
	args := make([]any, 0, 4)
	next := 1

	if ruleID := strings.TrimSpace(filter.RuleID); ruleID != "" {
		where = append(where, fmt.Sprintf("rule_id = $%d", next))
		args = append(args, ruleID)
		next++
	}
	if status := alerts.NormalizeInstanceStatus(filter.Status); status != "" {
		where = append(where, fmt.Sprintf("status = $%d", next))
		args = append(args, status)
		next++
	}
	if severity := alerts.NormalizeSeverity(filter.Severity); severity != "" {
		where = append(where, fmt.Sprintf("severity = $%d", next))
		args = append(args, severity)
		next++
	}

	sql := `SELECT id, rule_id, fingerprint, status, severity,
			labels, annotations, started_at, resolved_at, last_fired_at,
			suppressed_by, created_at, updated_at
		FROM alert_instances`
	if len(where) > 0 {
		sql += " WHERE " + strings.Join(where, " AND ")
	}
	sql += fmt.Sprintf(" ORDER BY updated_at DESC LIMIT $%d", next)
	args = append(args, limit)
	next++
	if filter.Offset > 0 {
		args = append(args, filter.Offset)
		sql += fmt.Sprintf(" OFFSET $%d", next)
	}

	rows, err := s.pool.Query(context.Background(), sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]alerts.AlertInstance, 0)
	for rows.Next() {
		inst, scanErr := scanAlertInstance(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, inst)
	}
	return out, rows.Err()
}

func (s *PostgresStore) UpdateAlertInstanceStatus(id, status string) (alerts.AlertInstance, error) {
	id = strings.TrimSpace(id)
	status = alerts.NormalizeInstanceStatus(status)
	if status == "" {
		return alerts.AlertInstance{}, errors.New("invalid alert instance status")
	}

	existing, ok, err := s.GetAlertInstance(id)
	if err != nil {
		return alerts.AlertInstance{}, err
	}
	if !ok {
		return alerts.AlertInstance{}, errors.New("alert instance not found")
	}
	if !alerts.CanTransitionInstanceStatus(existing.Status, status) {
		return alerts.AlertInstance{}, fmt.Errorf("cannot transition alert instance from %s to %s", existing.Status, status)
	}

	now := time.Now().UTC()
	var resolvedAt any = nil
	if status == alerts.InstanceStatusResolved {
		resolvedAt = now
	} else if existing.ResolvedAt != nil {
		resolvedAt = existing.ResolvedAt.UTC()
	}

	return scanAlertInstance(s.pool.QueryRow(context.Background(),
		`UPDATE alert_instances
		 SET status = $2, resolved_at = $3, updated_at = $4
		 WHERE id = $1
		 RETURNING id, rule_id, fingerprint, status, severity,
			labels, annotations, started_at, resolved_at, last_fired_at,
			suppressed_by, created_at, updated_at`,
		id, status, resolvedAt, now,
	))
}

func (s *PostgresStore) UpdateAlertInstanceLastFired(id string) error {
	now := time.Now().UTC()
	_, err := s.pool.Exec(context.Background(),
		`UPDATE alert_instances SET last_fired_at = $2, updated_at = $2 WHERE id = $1`,
		strings.TrimSpace(id), now,
	)
	return err
}

func (s *PostgresStore) DeleteAlertInstance(id string) error {
	tag, err := s.pool.Exec(context.Background(),
		`DELETE FROM alert_instances WHERE id = $1`,
		strings.TrimSpace(id),
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("alert instance not found")
	}
	return nil
}

func (s *PostgresStore) CreateAlertSilence(req alerts.CreateSilenceRequest) (alerts.AlertSilence, error) {
	now := time.Now().UTC()
	createdBy := strings.TrimSpace(req.CreatedBy)
	if createdBy == "" {
		createdBy = "owner"
	}

	matchersPayload, err := marshalStringMap(req.Matchers)
	if err != nil {
		return alerts.AlertSilence{}, err
	}

	return scanAlertSilence(s.pool.QueryRow(context.Background(),
		`INSERT INTO alert_silences (id, matchers, reason, created_by, starts_at, ends_at, created_at)
		 VALUES ($1, $2::jsonb, $3, $4, $5, $6, $7)
		 RETURNING id, matchers, reason, created_by, starts_at, ends_at, created_at`,
		idgen.New("sil"),
		matchersPayload,
		strings.TrimSpace(req.Reason),
		createdBy,
		req.StartsAt.UTC(),
		req.EndsAt.UTC(),
		now,
	))
}

func (s *PostgresStore) ListAlertSilences(limit int, activeOnly bool) ([]alerts.AlertSilence, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	sql := `SELECT id, matchers, reason, created_by, starts_at, ends_at, created_at FROM alert_silences`
	args := make([]any, 0, 2)
	if activeOnly {
		sql += ` WHERE starts_at <= $1 AND ends_at > $1`
		args = append(args, time.Now().UTC())
		sql += fmt.Sprintf(` ORDER BY ends_at DESC LIMIT $%d`, 2)
	} else {
		sql += ` ORDER BY created_at DESC LIMIT $1`
	}
	args = append(args, limit)

	rows, err := s.pool.Query(context.Background(), sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]alerts.AlertSilence, 0)
	for rows.Next() {
		silence, scanErr := scanAlertSilence(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, silence)
	}
	return out, rows.Err()
}

func (s *PostgresStore) GetAlertSilence(id string) (alerts.AlertSilence, bool, error) {
	silence, err := scanAlertSilence(s.pool.QueryRow(context.Background(),
		`SELECT id, matchers, reason, created_by, starts_at, ends_at, created_at
		 FROM alert_silences WHERE id = $1`,
		strings.TrimSpace(id),
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return alerts.AlertSilence{}, false, nil
		}
		return alerts.AlertSilence{}, false, err
	}
	return silence, true, nil
}

func (s *PostgresStore) DeleteAlertSilence(id string) error {
	tag, err := s.pool.Exec(context.Background(),
		`DELETE FROM alert_silences WHERE id = $1`,
		strings.TrimSpace(id),
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("alert silence not found")
	}
	return nil
}

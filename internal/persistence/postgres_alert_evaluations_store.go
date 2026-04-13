package persistence

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/idgen"
)

func (s *PostgresStore) RecordAlertEvaluation(ruleID string, evaluation alerts.Evaluation) (alerts.Evaluation, error) {
	ruleID = strings.TrimSpace(ruleID)

	evaluatedAt := evaluation.EvaluatedAt.UTC()
	if evaluatedAt.IsZero() {
		evaluatedAt = time.Now().UTC()
	}
	status := alerts.NormalizeEvaluationStatus(evaluation.Status)
	if status == "" {
		status = alerts.EvaluationStatusError
	}

	evaluationID := strings.TrimSpace(evaluation.ID)
	if evaluationID == "" {
		evaluationID = idgen.New("areval")
	}
	if evaluation.DurationMS < 0 {
		evaluation.DurationMS = 0
	}
	if evaluation.CandidateCount < 0 {
		evaluation.CandidateCount = 0
	}
	if evaluation.TriggeredCount < 0 {
		evaluation.TriggeredCount = 0
	}

	detailsPayload, err := marshalAnyMap(evaluation.Details)
	if err != nil {
		return alerts.Evaluation{}, err
	}

	tx, err := s.pool.Begin(context.Background())
	if err != nil {
		return alerts.Evaluation{}, err
	}
	defer tx.Rollback(context.Background())

	if _, err := tx.Exec(context.Background(),
		`INSERT INTO alert_evaluations (
			id,
			rule_id,
			status,
			evaluated_at,
			duration_ms,
			candidate_count,
			triggered_count,
			error,
			details
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb)`,
		evaluationID,
		ruleID,
		status,
		evaluatedAt,
		evaluation.DurationMS,
		evaluation.CandidateCount,
		evaluation.TriggeredCount,
		strings.TrimSpace(evaluation.Error),
		detailsPayload,
	); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return alerts.Evaluation{}, alerts.ErrRuleNotFound
		}
		return alerts.Evaluation{}, err
	}

	if _, err := tx.Exec(context.Background(),
		`UPDATE alert_rules
		 SET last_evaluated_at = $2
		 WHERE id = $1`,
		ruleID,
		evaluatedAt,
	); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return alerts.Evaluation{}, alerts.ErrRuleNotFound
		}
		return alerts.Evaluation{}, err
	}

	if err := tx.Commit(context.Background()); err != nil {
		return alerts.Evaluation{}, err
	}

	return alerts.Evaluation{
		ID:             evaluationID,
		RuleID:         ruleID,
		Status:         status,
		EvaluatedAt:    evaluatedAt,
		DurationMS:     evaluation.DurationMS,
		CandidateCount: evaluation.CandidateCount,
		TriggeredCount: evaluation.TriggeredCount,
		Error:          strings.TrimSpace(evaluation.Error),
		Details:        cloneAnyMap(evaluation.Details),
	}, nil
}

func (s *PostgresStore) ListAlertEvaluations(ruleID string, limit int) ([]alerts.Evaluation, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	ruleID = strings.TrimSpace(ruleID)

	rows, err := s.pool.Query(context.Background(),
		`SELECT
			id,
			rule_id,
			status,
			evaluated_at,
			duration_ms,
			candidate_count,
			triggered_count,
			error,
			details
		 FROM alert_evaluations
		 WHERE rule_id = $1
		 ORDER BY evaluated_at DESC
		 LIMIT $2`,
		ruleID,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]alerts.Evaluation, 0)
	for rows.Next() {
		evaluation, scanErr := scanAlertEvaluation(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, evaluation)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

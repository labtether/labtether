package persistence

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/labtether/labtether/internal/alerts"
)

func (s *PostgresStore) PageActiveAlertRules(ctx context.Context, cursor AlertRuleEvaluationCursor, limit int) (AlertRuleEvaluationPage, error) {
	if err := validateAlertRuleEvaluationPage(cursor, limit); err != nil {
		return AlertRuleEvaluationPage{}, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return AlertRuleEvaluationPage{}, err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	out := AlertRuleEvaluationPage{ThroughID: cursor.ThroughID, Rules: []alerts.Rule{}}
	if !cursor.Started {
		if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(id COLLATE "C"),'') FROM alert_rules WHERE status='active'`).Scan(&out.ThroughID); err != nil {
			return AlertRuleEvaluationPage{}, err
		}
	}
	rows, err := tx.Query(ctx, `SELECT id, name, description, status, kind, severity,
 target_scope, cooldown_seconds, reopen_after_seconds, evaluation_interval_seconds,
 window_seconds, condition, labels, metadata, created_by, created_at, updated_at,
 last_evaluated_at, last_auto_attempt_at
 FROM alert_rules WHERE status='active' AND id COLLATE "C" > $1 COLLATE "C"
 AND id COLLATE "C" <= $2 COLLATE "C" ORDER BY id COLLATE "C" LIMIT $3`, cursor.AfterID, out.ThroughID, limit+1)
	if err != nil {
		return AlertRuleEvaluationPage{}, err
	}
	for rows.Next() {
		rule, err := scanAlertRule(rows)
		if err != nil {
			rows.Close()
			return AlertRuleEvaluationPage{}, err
		}
		out.Rules = append(out.Rules, rule)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return AlertRuleEvaluationPage{}, err
	}
	out.Done = len(out.Rules) <= limit
	if len(out.Rules) > limit {
		out.Rules = out.Rules[:limit]
	}
	ids := make([]string, 0, len(out.Rules))
	for _, rule := range out.Rules {
		ids = append(ids, rule.ID)
	}
	targets, err := listAlertRuleTargetsBatchFrom(ctx, tx, ids)
	if err != nil {
		return AlertRuleEvaluationPage{}, err
	}
	for i := range out.Rules {
		out.Rules[i].Targets = targets[out.Rules[i].ID]
		if out.Rules[i].Targets == nil {
			out.Rules[i].Targets = []alerts.RuleTarget{}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return AlertRuleEvaluationPage{}, err
	}
	return out, nil
}

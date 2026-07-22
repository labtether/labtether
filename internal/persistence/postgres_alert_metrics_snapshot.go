package persistence

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/labtether/labtether/internal/telemetry"
)

// AlertMetricsSnapshot returns exact aggregate counts and at most
// maxRuleSeries latest per-rule evaluation timings in one bounded query. The
// rule limit applies only to labeled series; aggregate counts remain exact.
func (s *PostgresStore) AlertMetricsSnapshot(ctx context.Context, maxRuleSeries int) (AlertMetricsSnapshot, error) {
	if ctx == nil {
		return AlertMetricsSnapshot{}, fmt.Errorf("alert metric snapshot context is required")
	}
	if maxRuleSeries <= 0 || maxRuleSeries > telemetry.MaxAlertRuleMetricSeries {
		return AlertMetricsSnapshot{}, ErrAlertMetricSnapshotLimitExceeded
	}

	rows, err := s.pool.Query(ctx, `
		WITH active_rules AS MATERIALIZED (
			SELECT id, name
			  FROM alert_rules
			 WHERE status = 'active'
		), counts AS (
			SELECT
				(SELECT COUNT(*) FROM active_rules) AS active_rule_count,
				(SELECT COUNT(*) FROM alert_instances WHERE status = 'firing') AS firing_instance_count
		), limited_rules AS MATERIALIZED (
			SELECT id, name
			  FROM active_rules
			 ORDER BY id
			 LIMIT $1
		), latest_evaluations AS (
			SELECT rule.id AS rule_id, rule.name AS rule_name, evaluation.duration_ms
			  FROM limited_rules AS rule
			  JOIN LATERAL (
				SELECT duration_ms
				  FROM alert_evaluations
				 WHERE rule_id = rule.id
				 ORDER BY evaluated_at DESC, id DESC
				 LIMIT 1
			  ) AS evaluation ON TRUE
		)
		SELECT counts.active_rule_count,
		       counts.firing_instance_count,
		       latest_evaluations.rule_id,
		       latest_evaluations.rule_name,
		       latest_evaluations.duration_ms
		  FROM counts
		  LEFT JOIN latest_evaluations ON TRUE
		 ORDER BY latest_evaluations.rule_id`, maxRuleSeries)
	if err != nil {
		return AlertMetricsSnapshot{}, err
	}
	defer rows.Close()

	snapshot := AlertMetricsSnapshot{
		RuleEvaluations: make([]AlertRuleMetricSnapshot, 0, maxRuleSeries),
	}
	for rows.Next() {
		var (
			ruleID     sql.NullString
			ruleName   sql.NullString
			durationMS sql.NullInt64
		)
		if err := rows.Scan(
			&snapshot.ActiveRuleCount,
			&snapshot.FiringInstanceCount,
			&ruleID,
			&ruleName,
			&durationMS,
		); err != nil {
			return AlertMetricsSnapshot{}, err
		}
		if !ruleID.Valid {
			continue
		}
		snapshot.RuleEvaluations = append(snapshot.RuleEvaluations, AlertRuleMetricSnapshot{
			RuleID:     ruleID.String,
			RuleName:   ruleName.String,
			DurationMS: int(durationMS.Int64),
		})
	}
	if err := rows.Err(); err != nil {
		return AlertMetricsSnapshot{}, err
	}
	return snapshot, nil
}

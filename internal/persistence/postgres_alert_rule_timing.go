package persistence

import (
	"context"
	"strings"
	"time"
)

func (s *PostgresStore) ClaimAlertRuleEvaluation(ctx context.Context, id string, at time.Time) (bool, error) {
	if at.IsZero() {
		return false, nil
	}
	tag, err := s.pool.Exec(ctx, `UPDATE alert_rules SET last_auto_attempt_at=$2
 WHERE id=$1 AND status='active'
 AND (last_auto_attempt_at IS NULL OR last_auto_attempt_at + evaluation_interval_seconds * INTERVAL '1 second' <= $2)`, strings.TrimSpace(id), at.UTC())
	return err == nil && tag.RowsAffected() == 1, err
}

func (s *PostgresStore) AlertFingerprintTiming(ctx context.Context, ruleID, fingerprint string) (AlertFingerprintTiming, error) {
	out := AlertFingerprintTiming{}
	err := s.pool.QueryRow(ctx, `SELECT MAX(first_fired_at), MAX(resolved_at)
 FROM alert_instances WHERE rule_id=$1 AND fingerprint=$2 AND first_fired_at IS NOT NULL`,
		strings.TrimSpace(ruleID), strings.TrimSpace(fingerprint)).Scan(&out.LastFirstFiredAt, &out.LastResolvedAt)
	return out, err
}

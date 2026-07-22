package persistence

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/telemetry"
)

func TestPostgresAlertMetricsSnapshotExactCountsAndBoundedSeries(t *testing.T) {
	store := newTestPostgresStore(t)
	baseline, err := store.AlertMetricsSnapshot(context.Background(), telemetry.MaxAlertRuleMetricSeries)
	if err != nil {
		t.Fatalf("load baseline alert snapshot: %v", err)
	}
	if baseline.ActiveRuleCount == 0 && baseline.FiringInstanceCount == 0 && len(baseline.RuleEvaluations) != 0 {
		t.Fatalf("zero-count snapshot must still return one aggregate row and no rule series: %+v", baseline)
	}

	prefix := fmt.Sprintf("000-ltqa-alert-snapshot-%d", time.Now().UTC().UnixNano())
	now := time.Now().UTC()
	t.Cleanup(func() {
		_, _ = store.pool.Exec(context.Background(), `DELETE FROM alert_rules WHERE id LIKE $1`, prefix+"%")
	})

	if _, err := store.pool.Exec(context.Background(), `
		INSERT INTO alert_rules (
			id, name, description, status, kind, severity, target_scope,
			cooldown_seconds, reopen_after_seconds, evaluation_interval_seconds,
			window_seconds, condition, labels, metadata, created_by,
			created_at, updated_at, last_evaluated_at
		)
		SELECT $1 || '-rule-' || LPAD(series::text, 4, '0'),
		       'duplicate-name', '',
		       CASE WHEN series <= 502 THEN 'active' ELSE 'paused' END,
		       'metric_threshold', 'medium', 'global',
		       300, 60, 30, 300, '{}'::jsonb, '{}'::jsonb, '{}'::jsonb,
		       'owner', $2::timestamptz, $2::timestamptz,
		       CASE WHEN series <= 502 THEN $2::timestamptz ELSE NULL::timestamptz END
		  FROM generate_series(1, 503) AS series`, prefix, now); err != nil {
		t.Fatalf("seed alert rules: %v", err)
	}
	if _, err := store.pool.Exec(context.Background(), `
		INSERT INTO alert_evaluations (
			id, rule_id, status, evaluated_at, duration_ms,
			candidate_count, triggered_count, error, details
		)
		SELECT $1 || '-eval-' || LPAD(series::text, 4, '0') || '-a',
		       $1 || '-rule-' || LPAD(series::text, 4, '0'),
		       'ok', $2, series, 0, 0, '', '{}'::jsonb
		  FROM generate_series(1, 502) AS series`, prefix, now); err != nil {
		t.Fatalf("seed alert evaluations: %v", err)
	}
	if _, err := store.pool.Exec(context.Background(), `
		INSERT INTO alert_evaluations (
			id, rule_id, status, evaluated_at, duration_ms,
			candidate_count, triggered_count, error, details
		) VALUES ($1, $2, 'ok', $3, 999, 0, 0, '', '{}'::jsonb)`,
		prefix+"-eval-0001-z",
		prefix+"-rule-0001",
		now,
	); err != nil {
		t.Fatalf("seed equal-time tie evaluation: %v", err)
	}
	if _, err := store.pool.Exec(context.Background(), `
		INSERT INTO alert_instances (
			id, rule_id, fingerprint, status, severity, labels, annotations,
			started_at, last_fired_at, created_at, updated_at
		)
		SELECT $1 || '-instance-' || LPAD(series::text, 4, '0'),
		       $1 || '-rule-0001',
		       $1 || '-fingerprint-' || LPAD(series::text, 4, '0'),
		       CASE WHEN series <= 501 THEN 'firing' ELSE 'pending' END,
		       'medium', '{}'::jsonb, '{}'::jsonb, $2, $2, $2, $2
		  FROM generate_series(1, 502) AS series`, prefix, now); err != nil {
		t.Fatalf("seed alert instances: %v", err)
	}

	snapshot, err := store.AlertMetricsSnapshot(context.Background(), telemetry.MaxAlertRuleMetricSeries)
	if err != nil {
		t.Fatalf("load alert snapshot: %v", err)
	}
	if snapshot.ActiveRuleCount != baseline.ActiveRuleCount+502 {
		t.Fatalf("active rule count = %d, want baseline %d + 502", snapshot.ActiveRuleCount, baseline.ActiveRuleCount)
	}
	if snapshot.FiringInstanceCount != baseline.FiringInstanceCount+501 {
		t.Fatalf("firing count = %d, want baseline %d + 501", snapshot.FiringInstanceCount, baseline.FiringInstanceCount)
	}
	if len(snapshot.RuleEvaluations) != telemetry.MaxAlertRuleMetricSeries {
		t.Fatalf("rule series = %d, want cap %d", len(snapshot.RuleEvaluations), telemetry.MaxAlertRuleMetricSeries)
	}
	seen := make(map[string]struct{}, len(snapshot.RuleEvaluations))
	var foundTie bool
	for _, evaluation := range snapshot.RuleEvaluations {
		if _, duplicate := seen[evaluation.RuleID]; duplicate {
			t.Fatalf("duplicate rule series for ID %q", evaluation.RuleID)
		}
		seen[evaluation.RuleID] = struct{}{}
		if evaluation.RuleID == prefix+"-rule-0001" {
			foundTie = true
			if evaluation.RuleName != "duplicate-name" || evaluation.DurationMS != 999 {
				t.Fatalf("latest equal-time evaluation tie-break mismatch: %+v", evaluation)
			}
		}
	}
	if !foundTie {
		t.Fatalf("seeded duplicate-name rule missing from bounded snapshot")
	}

	if _, err := store.AlertMetricsSnapshot(context.Background(), telemetry.MaxAlertRuleMetricSeries+1); !errors.Is(err, ErrAlertMetricSnapshotLimitExceeded) {
		t.Fatalf("oversized rule budget error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.AlertMetricsSnapshot(ctx, telemetry.MaxAlertRuleMetricSeries); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled snapshot error = %v, want context.Canceled", err)
	}
}

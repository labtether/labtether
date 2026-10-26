package persistence

func schemaMigrationAlertEvaluationTiming() schemaMigration {
	return schemaMigration{Version: 104, Name: "alert_evaluation_timing", Statements: []string{
		`ALTER TABLE alert_rules ADD COLUMN last_auto_attempt_at TIMESTAMPTZ`,
		`ALTER TABLE alert_instances ADD COLUMN first_fired_at TIMESTAMPTZ`,
		`CREATE INDEX idx_alert_instances_known_fire ON alert_instances(rule_id, fingerprint, first_fired_at DESC) INCLUDE (resolved_at) WHERE first_fired_at IS NOT NULL`,
	}}
}

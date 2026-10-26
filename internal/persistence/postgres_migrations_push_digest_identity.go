package persistence

func schemaMigrationPushDigestSuppressionIdentity() schemaMigration {
	return schemaMigration{Version: 103, Name: "push_digest_suppression_identity", Statements: []string{
		`ALTER TABLE push_alert_digest_events
 ADD COLUMN rule_id TEXT,
 ADD COLUMN labels JSONB,
 ADD COLUMN due_at TIMESTAMPTZ,
 ADD COLUMN retry_count INTEGER NOT NULL DEFAULT 0 CHECK (retry_count BETWEEN 0 AND 8),
 ADD COLUMN delivery_generation BIGINT NOT NULL DEFAULT 0,
 ADD CONSTRAINT push_digest_rule_id_bound CHECK (rule_id IS NULL OR octet_length(rule_id) BETWEEN 1 AND 255),
 ADD CONSTRAINT push_digest_labels_bound CHECK (labels IS NULL OR (jsonb_typeof(labels) = 'object' AND octet_length(labels::text) <= 32768))`,
		`UPDATE push_alert_digest_events event SET due_at = state.due_at, retry_count = state.retry_count
 FROM push_alert_digest_states state WHERE event.push_device_id = state.push_device_id`,
		`ALTER TABLE push_alert_digest_events ALTER COLUMN due_at SET NOT NULL, ALTER COLUMN due_at SET DEFAULT NOW()`,
		`CREATE INDEX idx_push_digest_events_due ON push_alert_digest_events(push_device_id, due_at, created_at, id)`,
	}}
}

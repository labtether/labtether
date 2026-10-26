package persistence

// schemaMigrationPushRelayTransport stores whether a push credential is a raw
// APNs token ("direct") or a LabTether push relay registration id ("relay").
// Existing rows stay direct. The marker table records one-time automatic
// notification setup, such as the default relay channel, so it is not
// recreated after an operator deletes it.
func schemaMigrationPushRelayTransport() schemaMigration {
	return schemaMigration{Version: 106, Name: "push_relay_transport", Statements: []string{
		`ALTER TABLE push_devices ADD COLUMN IF NOT EXISTS push_transport TEXT NOT NULL DEFAULT 'direct'
 CHECK (push_transport IN ('direct', 'relay'))`,
		`ALTER TABLE live_activity_push_tokens ADD COLUMN IF NOT EXISTS push_transport TEXT NOT NULL DEFAULT 'direct'
 CHECK (push_transport IN ('direct', 'relay'))`,
		`CREATE TABLE IF NOT EXISTS notification_bootstrap_markers (
 name TEXT PRIMARY KEY CHECK (octet_length(name) BETWEEN 1 AND 128),
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
)`,
	}}
}

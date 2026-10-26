package persistence

func schemaMigrationAlertFiringScope() schemaMigration {
	return schemaMigration{Version: 105, Name: "alert_firing_scope", Statements: []string{
		`ALTER TABLE alert_instances ADD COLUMN firing_scope JSONB`,
	}}
}

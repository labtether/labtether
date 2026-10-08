package persistence

// Schema changes owned by the logs domain.

func schemaMigrationLogEventsSourceLevelTimestampIndex() schemaMigration {
	return schemaMigration{
		Version: 38,
		Name:    "log_events_source_level_timestamp_index",
		Statements: []string{
			`CREATE INDEX IF NOT EXISTS idx_log_events_source_level_timestamp ON log_events(source, level, timestamp DESC)`,
		},
	}
}

func schemaMigrationLogEventsDeadLetterErrorTimestampIndex() schemaMigration {
	return schemaMigration{
		Version: 39,
		Name:    "log_events_dead_letter_error_timestamp_index",
		Statements: []string{
			`CREATE INDEX IF NOT EXISTS idx_log_events_dead_letter_error_timestamp
				ON log_events(timestamp DESC)
				WHERE source = 'dead_letter' AND level = 'error'`,
		},
	}
}

func schemaMigrationLogEventsTimestampSourceIndex() schemaMigration {
	return schemaMigration{
		Version: 40,
		Name:    "log_events_timestamp_source_index",
		Statements: []string{
			`CREATE INDEX IF NOT EXISTS idx_log_events_timestamp_source ON log_events(timestamp DESC, source)`,
		},
	}
}

func schemaMigrationLogEventsSiteProjectionTimestampIndex() schemaMigration {
	return schemaMigration{
		Version: 41,
		Name:    "log_events_site_projection_timestamp_index",
		Statements: []string{
			`CREATE INDEX IF NOT EXISTS idx_log_events_site_projection_timestamp
				ON log_events ((NULLIF(BTRIM(fields->>'site_id'), '')), timestamp DESC)`,
		},
	}
}

func schemaMigrationAuditEventsIndexes() schemaMigration {
	return schemaMigration{
		Version: 73,
		Name:    "audit_events_indexes",
		Statements: []string{
			`CREATE INDEX IF NOT EXISTS idx_audit_events_actor_id ON audit_events (actor_id)`,
			`CREATE INDEX IF NOT EXISTS idx_audit_events_type ON audit_events (type)`,
		},
	}
}

func schemaMigrationSavedLogViewsOwnerScope() schemaMigration {
	return schemaMigration{
		Version: 76,
		Name:    "saved_log_views_owner_scope",
		Statements: []string{
			`ALTER TABLE saved_log_views ADD COLUMN IF NOT EXISTS owner_id TEXT NOT NULL DEFAULT ''`,
			`UPDATE saved_log_views SET owner_id = '' WHERE owner_id IS NULL`,
			`CREATE INDEX IF NOT EXISTS idx_saved_log_views_owner_updated ON saved_log_views(owner_id, updated_at DESC)`,
		},
	}
}

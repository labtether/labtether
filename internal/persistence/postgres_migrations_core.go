package persistence

// Schema changes owned by the core domain.

func schemaMigrationCoreSchema() schemaMigration {
	return schemaMigration{
		Version: 1,
		Name:    "core_schema",
		Statements: []string{
			`CREATE TABLE IF NOT EXISTS terminal_sessions (
					id TEXT PRIMARY KEY,
					actor_id TEXT NOT NULL,
					target TEXT NOT NULL,
					mode TEXT NOT NULL,
					status TEXT NOT NULL,
					created_at TIMESTAMPTZ NOT NULL,
					last_action_at TIMESTAMPTZ NOT NULL
				)`,
			`CREATE TABLE IF NOT EXISTS terminal_commands (
					id TEXT PRIMARY KEY,
					session_id TEXT NOT NULL REFERENCES terminal_sessions(id) ON DELETE CASCADE,
					actor_id TEXT NOT NULL,
					target TEXT NOT NULL,
					body TEXT NOT NULL,
					mode TEXT NOT NULL,
					status TEXT NOT NULL,
					output TEXT NOT NULL DEFAULT '',
					created_at TIMESTAMPTZ NOT NULL,
					updated_at TIMESTAMPTZ NOT NULL
				)`,
			`CREATE INDEX IF NOT EXISTS idx_terminal_commands_session_created ON terminal_commands(session_id, created_at DESC)`,
			`CREATE INDEX IF NOT EXISTS idx_terminal_commands_updated ON terminal_commands(updated_at DESC)`,
			`CREATE TABLE IF NOT EXISTS audit_events (
					id TEXT PRIMARY KEY,
					type TEXT NOT NULL,
					actor_id TEXT,
					target TEXT,
					session_id TEXT,
					command_id TEXT,
					decision TEXT,
					reason TEXT,
					details JSONB,
					timestamp TIMESTAMPTZ NOT NULL
				)`,
			`CREATE INDEX IF NOT EXISTS idx_audit_events_timestamp ON audit_events(timestamp DESC)`,
			`CREATE TABLE IF NOT EXISTS assets (
					id TEXT PRIMARY KEY,
					type TEXT NOT NULL,
					name TEXT NOT NULL,
					source TEXT NOT NULL,
					status TEXT NOT NULL,
					platform TEXT,
					metadata JSONB,
					created_at TIMESTAMPTZ NOT NULL,
					updated_at TIMESTAMPTZ NOT NULL,
					last_seen_at TIMESTAMPTZ NOT NULL
				)`,
			`CREATE INDEX IF NOT EXISTS idx_assets_last_seen ON assets(last_seen_at DESC)`,
			`CREATE TABLE IF NOT EXISTS asset_heartbeats (
					id TEXT PRIMARY KEY,
					asset_id TEXT NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
					source TEXT NOT NULL,
					status TEXT NOT NULL,
					metadata JSONB,
					received_at TIMESTAMPTZ NOT NULL
				)`,
			`CREATE INDEX IF NOT EXISTS idx_asset_heartbeats_asset_received ON asset_heartbeats(asset_id, received_at DESC)`,
			`CREATE TABLE IF NOT EXISTS metric_samples (
					id BIGSERIAL PRIMARY KEY,
					asset_id TEXT NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
					metric TEXT NOT NULL,
					unit TEXT NOT NULL,
					value DOUBLE PRECISION NOT NULL,
					collected_at TIMESTAMPTZ NOT NULL
				)`,
			`CREATE INDEX IF NOT EXISTS idx_metric_samples_asset_metric_time ON metric_samples(asset_id, metric, collected_at DESC)`,
			`CREATE INDEX IF NOT EXISTS idx_metric_samples_time ON metric_samples(collected_at DESC)`,
			`CREATE TABLE IF NOT EXISTS log_events (
					id TEXT PRIMARY KEY,
					asset_id TEXT,
					source TEXT NOT NULL,
					level TEXT NOT NULL,
					message TEXT NOT NULL,
					fields JSONB,
					timestamp TIMESTAMPTZ NOT NULL
				)`,
			`CREATE INDEX IF NOT EXISTS idx_log_events_timestamp ON log_events(timestamp DESC)`,
			`CREATE INDEX IF NOT EXISTS idx_log_events_source_timestamp ON log_events(source, timestamp DESC)`,
			`CREATE INDEX IF NOT EXISTS idx_log_events_asset_timestamp ON log_events(asset_id, timestamp DESC)`,
			`CREATE TABLE IF NOT EXISTS saved_log_views (
					id TEXT PRIMARY KEY,
					name TEXT NOT NULL,
					asset_id TEXT,
					source TEXT,
					level TEXT,
					search TEXT,
					window_value TEXT,
					created_at TIMESTAMPTZ NOT NULL,
					updated_at TIMESTAMPTZ NOT NULL
				)`,
			`CREATE INDEX IF NOT EXISTS idx_saved_log_views_updated ON saved_log_views(updated_at DESC)`,
			`CREATE TABLE IF NOT EXISTS action_runs (
					id TEXT PRIMARY KEY,
					type TEXT NOT NULL,
					actor_id TEXT NOT NULL,
					target TEXT,
					command TEXT,
					connector_id TEXT,
					action_id TEXT,
					params JSONB,
					dry_run BOOLEAN NOT NULL DEFAULT FALSE,
					status TEXT NOT NULL,
					output TEXT NOT NULL DEFAULT '',
					error TEXT NOT NULL DEFAULT '',
					created_at TIMESTAMPTZ NOT NULL,
					updated_at TIMESTAMPTZ NOT NULL,
					completed_at TIMESTAMPTZ
				)`,
			`CREATE INDEX IF NOT EXISTS idx_action_runs_updated ON action_runs(updated_at DESC)`,
			`CREATE INDEX IF NOT EXISTS idx_action_runs_status ON action_runs(status, updated_at DESC)`,
			`CREATE TABLE IF NOT EXISTS action_run_steps (
					id TEXT PRIMARY KEY,
					run_id TEXT NOT NULL REFERENCES action_runs(id) ON DELETE CASCADE,
					name TEXT NOT NULL,
					status TEXT NOT NULL,
					output TEXT NOT NULL DEFAULT '',
					error TEXT NOT NULL DEFAULT '',
					created_at TIMESTAMPTZ NOT NULL,
					updated_at TIMESTAMPTZ NOT NULL
				)`,
			`CREATE INDEX IF NOT EXISTS idx_action_run_steps_run_created ON action_run_steps(run_id, created_at ASC)`,
			`CREATE TABLE IF NOT EXISTS update_plans (
					id TEXT PRIMARY KEY,
					name TEXT NOT NULL,
					description TEXT NOT NULL DEFAULT '',
					targets JSONB NOT NULL,
					scopes JSONB NOT NULL,
					default_dry_run BOOLEAN NOT NULL DEFAULT TRUE,
					created_at TIMESTAMPTZ NOT NULL,
					updated_at TIMESTAMPTZ NOT NULL
				)`,
			`CREATE INDEX IF NOT EXISTS idx_update_plans_updated ON update_plans(updated_at DESC)`,
			`CREATE TABLE IF NOT EXISTS update_runs (
					id TEXT PRIMARY KEY,
					plan_id TEXT NOT NULL REFERENCES update_plans(id) ON DELETE CASCADE,
					plan_name TEXT NOT NULL,
					actor_id TEXT NOT NULL,
					dry_run BOOLEAN NOT NULL DEFAULT TRUE,
					status TEXT NOT NULL,
					summary TEXT NOT NULL DEFAULT '',
					error TEXT NOT NULL DEFAULT '',
					results JSONB,
					created_at TIMESTAMPTZ NOT NULL,
					updated_at TIMESTAMPTZ NOT NULL,
					completed_at TIMESTAMPTZ
				)`,
			`CREATE INDEX IF NOT EXISTS idx_update_runs_updated ON update_runs(updated_at DESC)`,
			`CREATE INDEX IF NOT EXISTS idx_update_runs_status ON update_runs(status, updated_at DESC)`,
		},
	}
}

func schemaMigrationAssetTags() schemaMigration {
	return schemaMigration{
		Version: 28,
		Name:    "asset_tags",
		Statements: []string{
			`ALTER TABLE assets ADD COLUMN IF NOT EXISTS tags JSONB NOT NULL DEFAULT '[]'::jsonb`,
		},
	}
}

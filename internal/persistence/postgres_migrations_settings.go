package persistence

// Schema changes owned by the settings domain.

func schemaMigrationRetentionSettings() schemaMigration {
	return schemaMigration{
		Version: 2,
		Name:    "retention_settings",
		Statements: []string{
			`CREATE TABLE IF NOT EXISTS retention_settings (
					id TEXT PRIMARY KEY,
					logs_window TEXT NOT NULL,
					metrics_window TEXT NOT NULL,
					audit_window TEXT NOT NULL,
					terminal_window TEXT NOT NULL,
					action_runs_window TEXT NOT NULL,
					update_runs_window TEXT NOT NULL,
					updated_at TIMESTAMPTZ NOT NULL
				)`,
		},
	}
}

func schemaMigrationRuntimeSettings() schemaMigration {
	return schemaMigration{
		Version: 4,
		Name:    "runtime_settings",
		Statements: []string{
			`CREATE TABLE IF NOT EXISTS runtime_settings (
					key TEXT PRIMARY KEY,
					value TEXT NOT NULL,
					updated_at TIMESTAMPTZ NOT NULL
				)`,
			`CREATE INDEX IF NOT EXISTS idx_runtime_settings_updated_at ON runtime_settings(updated_at DESC)`,
		},
	}
}

func schemaMigrationSystemSettings() schemaMigration {
	return schemaMigration{
		Version: 55,
		Name:    "system_settings",
		Statements: []string{
			`CREATE TABLE IF NOT EXISTS system_settings (
				key        TEXT PRIMARY KEY,
				value      JSONB NOT NULL,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
			)`,
		},
	}
}

func schemaMigrationWorkspaceTabPanelSizes() schemaMigration {
	return schemaMigration{
		Version: 58,
		Name:    "workspace_tab_panel_sizes",
		Statements: []string{
			`ALTER TABLE terminal_workspace_tabs ADD COLUMN IF NOT EXISTS panel_sizes JSONB`,
		},
	}
}

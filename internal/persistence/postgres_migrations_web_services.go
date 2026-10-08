package persistence

// Schema changes owned by the web services domain.

func schemaMigrationWebServicesManualAndOverrides() schemaMigration {
	return schemaMigration{
		Version: 36,
		Name:    "web_services_manual_and_overrides",
		Statements: []string{
			`CREATE TABLE IF NOT EXISTS web_services_manual (
				id TEXT PRIMARY KEY,
				host_asset_id TEXT NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
				name TEXT NOT NULL,
				category TEXT NOT NULL,
				url TEXT NOT NULL,
				icon_key TEXT NOT NULL DEFAULT '',
				metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
				created_at TIMESTAMPTZ NOT NULL,
				updated_at TIMESTAMPTZ NOT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS idx_web_services_manual_host_updated
				ON web_services_manual(host_asset_id, updated_at DESC)`,
			`CREATE TABLE IF NOT EXISTS web_service_overrides (
				host_asset_id TEXT NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
				service_id TEXT NOT NULL,
				name_override TEXT NOT NULL DEFAULT '',
				category_override TEXT NOT NULL DEFAULT '',
				url_override TEXT NOT NULL DEFAULT '',
				icon_key_override TEXT NOT NULL DEFAULT '',
				hidden BOOLEAN NOT NULL DEFAULT FALSE,
				updated_at TIMESTAMPTZ NOT NULL,
				PRIMARY KEY (host_asset_id, service_id)
			)`,
			`CREATE INDEX IF NOT EXISTS idx_web_service_overrides_host_updated
				ON web_service_overrides(host_asset_id, updated_at DESC)`,
		},
	}
}

func schemaMigrationWebServiceOverridesTags() schemaMigration {
	return schemaMigration{
		Version: 43,
		Name:    "web_service_overrides_tags",
		Statements: []string{
			`ALTER TABLE web_service_overrides ADD COLUMN IF NOT EXISTS tags_override TEXT NOT NULL DEFAULT ''`,
		},
	}
}

func schemaMigrationWebServiceAltUrlsAndGrouping() schemaMigration {
	return schemaMigration{
		Version: 44,
		Name:    "web_service_alt_urls_and_grouping",
		Statements: []string{
			`CREATE TABLE IF NOT EXISTS web_service_alt_urls (
				id TEXT PRIMARY KEY,
				web_service_id TEXT NOT NULL,
				url TEXT NOT NULL,
				source TEXT NOT NULL DEFAULT 'auto',
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				UNIQUE (web_service_id, url)
			)`,
			`CREATE INDEX idx_web_service_alt_urls_service ON web_service_alt_urls (web_service_id)`,
			`CREATE TABLE IF NOT EXISTS web_service_url_grouping_settings (
				id TEXT PRIMARY KEY,
				setting_key TEXT NOT NULL UNIQUE,
				setting_value TEXT NOT NULL DEFAULT '',
				updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
			)`,
			`CREATE TABLE IF NOT EXISTS web_service_never_group_rules (
				id TEXT PRIMARY KEY,
				url_a TEXT NOT NULL,
				url_b TEXT NOT NULL,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				UNIQUE (url_a, url_b)
			)`,
			`CREATE INDEX idx_web_service_never_group_rules_pair ON web_service_never_group_rules (url_a, url_b)`,
		},
	}
}

func schemaMigrationMigrateMergeSettingsToUrlGrouping() schemaMigration {
	return schemaMigration{
		Version: 45,
		Name:    "migrate_merge_settings_to_url_grouping",
		Statements: []string{
			`INSERT INTO web_service_url_grouping_settings (id, setting_key, setting_value, updated_at)
			SELECT 'grpset-mode', 'grouping_mode', COALESCE(value, 'balanced'), updated_at
			FROM runtime_settings WHERE key = 'services.merge_mode'
			ON CONFLICT (setting_key) DO NOTHING`,

			`INSERT INTO web_service_url_grouping_settings (id, setting_key, setting_value, updated_at)
			SELECT 'grpset-sens', 'sensitivity', COALESCE(value, '85'), updated_at
			FROM runtime_settings WHERE key = 'services.merge_confidence_threshold'
			ON CONFLICT (setting_key) DO NOTHING`,

			`INSERT INTO web_service_url_grouping_settings (id, setting_key, setting_value, updated_at)
			SELECT 'grpset-alias', 'alias_rules', COALESCE(value, ''), updated_at
			FROM runtime_settings WHERE key = 'services.merge_alias_rules'
			ON CONFLICT (setting_key) DO NOTHING`,

			`DELETE FROM runtime_settings WHERE key IN (
				'services.merge_mode',
				'services.merge_confidence_threshold',
				'services.merge_dry_run',
				'services.merge_alias_rules',
				'services.force_merge_rules',
				'services.never_merge_rules'
			)`,
		},
	}
}

func schemaMigrationWebServicesManualNullableHost() schemaMigration {
	return schemaMigration{
		Version: 59,
		Name:    "web_services_manual_nullable_host",
		Statements: []string{
			`ALTER TABLE web_services_manual ALTER COLUMN host_asset_id DROP NOT NULL`,
			`ALTER TABLE web_services_manual DROP CONSTRAINT IF EXISTS web_services_manual_host_asset_id_fkey`,
			`ALTER TABLE web_services_manual ADD CONSTRAINT web_services_manual_host_asset_id_fkey FOREIGN KEY (host_asset_id) REFERENCES assets(id) ON DELETE SET NULL`,
		},
	}
}

package persistence

// Schema changes owned by the groups domain.

func schemaMigrationSitesAndAssetSiteFk() schemaMigration {
	return schemaMigration{
		Version: 3,
		Name:    "sites_and_asset_site_fk",
		Statements: []string{
			`CREATE TABLE IF NOT EXISTS groupmaintenance (
					id TEXT PRIMARY KEY,
					name TEXT NOT NULL,
					code TEXT NOT NULL UNIQUE,
					timezone TEXT,
					location TEXT,
					status TEXT NOT NULL,
					metadata JSONB,
					created_at TIMESTAMPTZ NOT NULL,
					updated_at TIMESTAMPTZ NOT NULL
				)`,
			`ALTER TABLE assets ADD COLUMN IF NOT EXISTS site_id TEXT REFERENCES groupmaintenance(id) ON DELETE SET NULL`,
			`CREATE INDEX IF NOT EXISTS idx_assets_site_last_seen ON assets(site_id, last_seen_at DESC)`,
		},
	}
}

func schemaMigrationSiteGeoAndMaintenanceWindows() schemaMigration {
	return schemaMigration{
		Version: 5,
		Name:    "site_geo_and_maintenance_windows",
		Statements: []string{
			`ALTER TABLE groupmaintenance ADD COLUMN IF NOT EXISTS latitude DOUBLE PRECISION`,
			`ALTER TABLE groupmaintenance ADD COLUMN IF NOT EXISTS longitude DOUBLE PRECISION`,
			`ALTER TABLE groupmaintenance ADD COLUMN IF NOT EXISTS geo_label TEXT`,
			`CREATE TABLE IF NOT EXISTS site_maintenance_windows (
					id TEXT PRIMARY KEY,
					site_id TEXT NOT NULL REFERENCES groupmaintenance(id) ON DELETE CASCADE,
					name TEXT NOT NULL,
					start_at TIMESTAMPTZ NOT NULL,
					end_at TIMESTAMPTZ NOT NULL,
					suppress_alerts BOOLEAN NOT NULL DEFAULT TRUE,
					block_actions BOOLEAN NOT NULL DEFAULT FALSE,
					block_updates BOOLEAN NOT NULL DEFAULT FALSE,
					created_at TIMESTAMPTZ NOT NULL,
					updated_at TIMESTAMPTZ NOT NULL,
					CHECK (end_at > start_at)
				)`,
			`CREATE INDEX IF NOT EXISTS idx_site_maintenance_windows_site_start ON site_maintenance_windows(site_id, start_at DESC)`,
			`CREATE INDEX IF NOT EXISTS idx_site_maintenance_windows_site_active ON site_maintenance_windows(site_id, start_at, end_at)`,
		},
	}
}

func schemaMigrationSiteProfilesFailoverPairs() schemaMigration {
	return schemaMigration{
		Version: 16,
		Name:    "site_profiles_failover_pairs",
		Statements: []string{
			`CREATE TABLE IF NOT EXISTS site_profiles (
				id TEXT PRIMARY KEY,
				name TEXT NOT NULL,
				description TEXT DEFAULT '',
				config JSONB NOT NULL DEFAULT '{}',
				created_at TIMESTAMPTZ DEFAULT now(),
				updated_at TIMESTAMPTZ DEFAULT now()
			)`,
			`CREATE TABLE IF NOT EXISTS site_profile_assignments (
				id TEXT PRIMARY KEY,
				site_id TEXT NOT NULL REFERENCES groupmaintenance(id) ON DELETE CASCADE,
				profile_id TEXT NOT NULL REFERENCES site_profiles(id) ON DELETE CASCADE,
				assigned_by TEXT DEFAULT '',
				assigned_at TIMESTAMPTZ DEFAULT now(),
				UNIQUE(site_id)
			)`,
			`CREATE TABLE IF NOT EXISTS site_profile_drift_checks (
				id TEXT PRIMARY KEY,
				site_id TEXT NOT NULL REFERENCES groupmaintenance(id) ON DELETE CASCADE,
				profile_id TEXT NOT NULL REFERENCES site_profiles(id) ON DELETE CASCADE,
				status TEXT NOT NULL CHECK (status IN ('compliant','drifted')),
				drift_details JSONB DEFAULT '{}',
				checked_at TIMESTAMPTZ DEFAULT now()
			)`,
			`CREATE INDEX IF NOT EXISTS idx_drift_site_time ON site_profile_drift_checks(site_id, checked_at DESC)`,
			`CREATE TABLE IF NOT EXISTS site_failover_pairs (
				id TEXT PRIMARY KEY,
				primary_site_id TEXT NOT NULL REFERENCES groupmaintenance(id) ON DELETE CASCADE,
				backup_site_id TEXT NOT NULL REFERENCES groupmaintenance(id) ON DELETE CASCADE,
				name TEXT NOT NULL DEFAULT '',
				required_capabilities JSONB DEFAULT '{}',
				readiness_score INT DEFAULT 0,
				last_checked_at TIMESTAMPTZ,
				created_at TIMESTAMPTZ DEFAULT now(),
				updated_at TIMESTAMPTZ DEFAULT now(),
				UNIQUE(primary_site_id, backup_site_id),
				CHECK(primary_site_id != backup_site_id)
			)`,
		},
	}
}

// Device hierarchy: groups table, site migration and link suggestions.
func schemaMigrationGroupsTableAndSiteMigration() schemaMigration {
	return schemaMigration{
		Version: 46,
		Name:    "groups_table_and_site_migration",
		Statements: []string{
			// 1. Create the groups table with hierarchical self-reference.
			`CREATE TABLE IF NOT EXISTS groups (
				id TEXT PRIMARY KEY,
				name TEXT NOT NULL,
				slug TEXT NOT NULL,
				parent_group_id TEXT REFERENCES groups(id) ON DELETE CASCADE,
				icon TEXT NOT NULL DEFAULT '',
				sort_order INT NOT NULL DEFAULT 0,
				timezone TEXT NOT NULL DEFAULT '',
				location TEXT NOT NULL DEFAULT '',
				latitude DOUBLE PRECISION,
				longitude DOUBLE PRECISION,
				metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
				created_at TIMESTAMPTZ NOT NULL,
				updated_at TIMESTAMPTZ NOT NULL,
				UNIQUE (parent_group_id, slug)
			)`,
			`CREATE INDEX IF NOT EXISTS idx_groups_parent ON groups(parent_group_id)`,

			// 2. Migrate existing groupmaintenance into groups.
			`INSERT INTO groups (id, name, slug, parent_group_id, icon, sort_order, timezone, location, latitude, longitude, metadata, created_at, updated_at)
			SELECT id, name, code, NULL, '', 0, COALESCE(timezone, ''), COALESCE(location, ''), latitude, longitude, COALESCE(metadata, '{}'::jsonb), created_at, updated_at
			FROM groupmaintenance
			ON CONFLICT (id) DO NOTHING`,

			// 3. Rename assets.site_id -> assets.group_id.
			`ALTER TABLE assets RENAME COLUMN site_id TO group_id`,

			// 4. Drop old index on site_id, create new index on group_id.
			`DROP INDEX IF EXISTS idx_assets_site_last_seen`,
			`CREATE INDEX IF NOT EXISTS idx_assets_group_last_seen ON assets(group_id, last_seen_at DESC)`,

			// 5. Drop old FK to groupmaintenance, add new FK to groups.
			`ALTER TABLE assets DROP CONSTRAINT IF EXISTS assets_site_id_fkey`,
			`ALTER TABLE assets ADD CONSTRAINT assets_group_id_fkey FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE SET NULL`,

			// 6. Create asset_link_suggestions table.
			`CREATE TABLE IF NOT EXISTS asset_link_suggestions (
				id TEXT PRIMARY KEY,
				source_asset_id TEXT NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
				target_asset_id TEXT NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
				match_reason TEXT NOT NULL,
				confidence DOUBLE PRECISION NOT NULL DEFAULT 0,
				status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'accepted', 'dismissed')),
				created_at TIMESTAMPTZ NOT NULL,
				resolved_at TIMESTAMPTZ,
				resolved_by TEXT,
				UNIQUE (source_asset_id, target_asset_id),
				CHECK (source_asset_id != target_asset_id)
			)`,
			`CREATE INDEX IF NOT EXISTS idx_asset_link_suggestions_status ON asset_link_suggestions(status)`,

			// 7. Update dependency relationship_type CHECK to include 'contains'.
			`ALTER TABLE asset_dependencies DROP CONSTRAINT asset_dependencies_relationship_type_check`,
			`ALTER TABLE asset_dependencies ADD CONSTRAINT asset_dependencies_relationship_type_check CHECK (relationship_type IN ('runs_on', 'hosted_on', 'depends_on', 'provides_to', 'connected_to', 'contains'))`,

			// 8. Drop FKs from other tables that reference groupmaintenance before dropping groupmaintenance.
			// alert_rule_targets.site_id
			`ALTER TABLE alert_rule_targets DROP CONSTRAINT IF EXISTS alert_rule_targets_site_id_fkey`,
			`DROP INDEX IF EXISTS idx_alert_rule_targets_site`,
			`DROP INDEX IF EXISTS idx_alert_rule_targets_rule_site_unique`,
			// incidents.site_id
			`ALTER TABLE incidents DROP CONSTRAINT IF EXISTS incidents_site_id_fkey`,

			// 9. Drop legacy site-coupled tables (dependents first).
			`DROP TABLE IF EXISTS maintenance_overrides`,
			`DROP TABLE IF EXISTS site_maintenance_windows`,
			`DROP TABLE IF EXISTS site_profile_drift_checks`,
			`DROP TABLE IF EXISTS site_profile_assignments`,
			`DROP TABLE IF EXISTS site_failover_pairs`,
			`DROP TABLE IF EXISTS site_reliability_history`,
			`DROP TABLE IF EXISTS groupmaintenance`,
		},
	}
}

func schemaMigrationGroupsJumpChain() schemaMigration {
	return schemaMigration{
		Version: 57,
		Name:    "groups_jump_chain",
		Statements: []string{
			`ALTER TABLE groups ADD COLUMN IF NOT EXISTS jump_chain JSONB`,
		},
	}
}

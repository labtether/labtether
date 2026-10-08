package persistence

// Schema changes owned by the group compatibility domain.

func schemaMigrationGroupsSiteCompatibilityRestore() schemaMigration {
	return schemaMigration{
		Version: 49,
		Name:    "groups_site_compatibility_restore",
		Statements: []string{
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
				geo_label TEXT,
				status TEXT NOT NULL DEFAULT 'active',
				created_at TIMESTAMPTZ NOT NULL,
				updated_at TIMESTAMPTZ NOT NULL,
				UNIQUE (parent_group_id, slug)
			)`,
			`CREATE INDEX IF NOT EXISTS idx_groups_parent ON groups(parent_group_id)`,
			`DO $$
			BEGIN
				IF EXISTS (
					SELECT 1
					FROM information_schema.tables
					WHERE table_schema = current_schema() AND table_name = 'groupmaintenance'
				) THEN
					INSERT INTO groups (
						id,
						name,
						slug,
						parent_group_id,
						icon,
						sort_order,
						timezone,
						location,
						latitude,
						longitude,
						metadata,
						geo_label,
						status,
						created_at,
						updated_at
					)
					SELECT
						id,
						name,
						code,
						NULL,
						'',
						0,
						COALESCE(timezone, ''),
						COALESCE(location, ''),
						latitude,
						longitude,
						COALESCE(metadata, '{}'::jsonb),
						NULL,
						'active',
						created_at,
						updated_at
					FROM groupmaintenance
					ON CONFLICT (id) DO UPDATE SET
						name = EXCLUDED.name,
						slug = EXCLUDED.slug,
						timezone = EXCLUDED.timezone,
						location = EXCLUDED.location,
						latitude = EXCLUDED.latitude,
						longitude = EXCLUDED.longitude,
						metadata = EXCLUDED.metadata,
						updated_at = EXCLUDED.updated_at;
				END IF;
			END $$`,
			`ALTER TABLE groups ADD COLUMN IF NOT EXISTS geo_label TEXT`,
			`ALTER TABLE groups ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'active'`,
			`DO $$
			BEGIN
				IF EXISTS (
					SELECT 1
					FROM information_schema.columns
					WHERE table_schema = current_schema()
					  AND table_name = 'assets'
					  AND column_name = 'site_id'
				) AND NOT EXISTS (
					SELECT 1
					FROM information_schema.columns
					WHERE table_schema = current_schema()
					  AND table_name = 'assets'
					  AND column_name = 'group_id'
				) THEN
					EXECUTE 'ALTER TABLE assets RENAME COLUMN site_id TO group_id';
				END IF;
			END $$`,
			`DROP INDEX IF EXISTS idx_assets_site_last_seen`,
			`CREATE INDEX IF NOT EXISTS idx_assets_group_last_seen ON assets(group_id, last_seen_at DESC)`,
			`ALTER TABLE assets DROP CONSTRAINT IF EXISTS assets_group_id_fkey`,
			`ALTER TABLE assets
			 ADD CONSTRAINT assets_group_id_fkey
			 FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE SET NULL`,

			`CREATE TABLE IF NOT EXISTS site_maintenance_windows (
				id TEXT PRIMARY KEY,
				site_id TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
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

			`CREATE TABLE IF NOT EXISTS maintenance_overrides (
				id TEXT PRIMARY KEY,
				maintenance_window_id TEXT NOT NULL REFERENCES site_maintenance_windows(id) ON DELETE CASCADE,
				override_type TEXT NOT NULL CHECK (override_type IN ('action','update')),
				reason TEXT NOT NULL,
				reference_id TEXT DEFAULT '',
				approved_by TEXT DEFAULT '',
				created_at TIMESTAMPTZ DEFAULT now()
			)`,
			`CREATE INDEX IF NOT EXISTS idx_maint_overrides_window ON maintenance_overrides(maintenance_window_id)`,

			`CREATE TABLE IF NOT EXISTS site_reliability_history (
				id TEXT PRIMARY KEY,
				site_id TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
				score INT NOT NULL,
				grade TEXT NOT NULL,
				factors JSONB NOT NULL DEFAULT '{}',
				window_hours INT NOT NULL DEFAULT 24,
				computed_at TIMESTAMPTZ NOT NULL DEFAULT now()
			)`,
			`CREATE INDEX IF NOT EXISTS idx_rel_hist_site_time ON site_reliability_history(site_id, computed_at DESC)`,

			`CREATE TABLE IF NOT EXISTS site_profile_assignments (
				id TEXT PRIMARY KEY,
				site_id TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
				profile_id TEXT NOT NULL REFERENCES site_profiles(id) ON DELETE CASCADE,
				assigned_by TEXT DEFAULT '',
				assigned_at TIMESTAMPTZ DEFAULT now(),
				UNIQUE(site_id)
			)`,
			`CREATE TABLE IF NOT EXISTS site_profile_drift_checks (
				id TEXT PRIMARY KEY,
				site_id TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
				profile_id TEXT NOT NULL REFERENCES site_profiles(id) ON DELETE CASCADE,
				status TEXT NOT NULL CHECK (status IN ('compliant','drifted')),
				drift_details JSONB DEFAULT '{}',
				checked_at TIMESTAMPTZ DEFAULT now()
			)`,
			`CREATE INDEX IF NOT EXISTS idx_drift_site_time ON site_profile_drift_checks(site_id, checked_at DESC)`,

			`CREATE TABLE IF NOT EXISTS site_failover_pairs (
				id TEXT PRIMARY KEY,
				primary_site_id TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
				backup_site_id TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
				name TEXT NOT NULL DEFAULT '',
				required_capabilities JSONB DEFAULT '{}',
				readiness_score INT DEFAULT 0,
				last_checked_at TIMESTAMPTZ,
				created_at TIMESTAMPTZ DEFAULT now(),
				updated_at TIMESTAMPTZ DEFAULT now(),
				UNIQUE(primary_site_id, backup_site_id),
				CHECK(primary_site_id != backup_site_id)
			)`,

			`UPDATE alert_rule_targets art
			 SET site_id = NULL
			 WHERE site_id IS NOT NULL AND NOT EXISTS (
			 	SELECT 1 FROM groups g WHERE g.id = art.site_id
			 )`,
			`UPDATE incidents inc
			 SET site_id = NULL
			 WHERE site_id IS NOT NULL AND NOT EXISTS (
			 	SELECT 1 FROM groups g WHERE g.id = inc.site_id
			 )`,
			`ALTER TABLE alert_rule_targets DROP CONSTRAINT IF EXISTS alert_rule_targets_site_id_fkey`,
			`ALTER TABLE alert_rule_targets
			 ADD CONSTRAINT alert_rule_targets_site_id_fkey
			 FOREIGN KEY (site_id) REFERENCES groups(id) ON DELETE SET NULL`,
			`CREATE INDEX IF NOT EXISTS idx_alert_rule_targets_site ON alert_rule_targets(site_id) WHERE site_id IS NOT NULL`,
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_alert_rule_targets_rule_site_unique ON alert_rule_targets(rule_id, site_id) WHERE site_id IS NOT NULL`,
			`ALTER TABLE incidents DROP CONSTRAINT IF EXISTS incidents_site_id_fkey`,
			`ALTER TABLE incidents
			 ADD CONSTRAINT incidents_site_id_fkey
			 FOREIGN KEY (site_id) REFERENCES groups(id) ON DELETE SET NULL`,
		},
	}
}

func schemaMigrationGroupIdSiteCompatibilityRepair() schemaMigration {
	return schemaMigration{
		Version: 50,
		Name:    "group_id_site_compatibility_repair",
		Statements: []string{
			`CREATE TABLE IF NOT EXISTS site_maintenance_windows (
				id TEXT PRIMARY KEY,
				site_id TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
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

			`CREATE TABLE IF NOT EXISTS maintenance_overrides (
				id TEXT PRIMARY KEY,
				maintenance_window_id TEXT NOT NULL REFERENCES site_maintenance_windows(id) ON DELETE CASCADE,
				override_type TEXT NOT NULL CHECK (override_type IN ('action','update')),
				reason TEXT NOT NULL,
				reference_id TEXT DEFAULT '',
				approved_by TEXT DEFAULT '',
				created_at TIMESTAMPTZ DEFAULT now()
			)`,
			`CREATE INDEX IF NOT EXISTS idx_maint_overrides_window ON maintenance_overrides(maintenance_window_id)`,

			`CREATE TABLE IF NOT EXISTS site_reliability_history (
				id TEXT PRIMARY KEY,
				site_id TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
				score INT NOT NULL,
				grade TEXT NOT NULL,
				factors JSONB NOT NULL DEFAULT '{}',
				window_hours INT NOT NULL DEFAULT 24,
				computed_at TIMESTAMPTZ NOT NULL DEFAULT now()
			)`,
			`CREATE INDEX IF NOT EXISTS idx_rel_hist_site_time ON site_reliability_history(site_id, computed_at DESC)`,

			`CREATE TABLE IF NOT EXISTS site_profile_assignments (
				id TEXT PRIMARY KEY,
				site_id TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
				profile_id TEXT NOT NULL REFERENCES site_profiles(id) ON DELETE CASCADE,
				assigned_by TEXT DEFAULT '',
				assigned_at TIMESTAMPTZ DEFAULT now(),
				UNIQUE(site_id)
			)`,
			`CREATE TABLE IF NOT EXISTS site_profile_drift_checks (
				id TEXT PRIMARY KEY,
				site_id TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
				profile_id TEXT NOT NULL REFERENCES site_profiles(id) ON DELETE CASCADE,
				status TEXT NOT NULL CHECK (status IN ('compliant','drifted')),
				drift_details JSONB DEFAULT '{}',
				checked_at TIMESTAMPTZ DEFAULT now()
			)`,
			`CREATE INDEX IF NOT EXISTS idx_drift_site_time ON site_profile_drift_checks(site_id, checked_at DESC)`,

			`CREATE TABLE IF NOT EXISTS site_failover_pairs (
				id TEXT PRIMARY KEY,
				primary_site_id TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
				backup_site_id TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
				name TEXT NOT NULL DEFAULT '',
				required_capabilities JSONB DEFAULT '{}',
				readiness_score INT DEFAULT 0,
				last_checked_at TIMESTAMPTZ,
				created_at TIMESTAMPTZ DEFAULT now(),
				updated_at TIMESTAMPTZ DEFAULT now(),
				UNIQUE(primary_site_id, backup_site_id),
				CHECK(primary_site_id != backup_site_id)
			)`,

			`UPDATE alert_rule_targets art
			 SET site_id = NULL
			 WHERE site_id IS NOT NULL AND NOT EXISTS (
			 	SELECT 1 FROM groups g WHERE g.id = art.site_id
			 )`,
			`UPDATE incidents inc
			 SET site_id = NULL
			 WHERE site_id IS NOT NULL AND NOT EXISTS (
			 	SELECT 1 FROM groups g WHERE g.id = inc.site_id
			 )`,
			`ALTER TABLE alert_rule_targets DROP CONSTRAINT IF EXISTS alert_rule_targets_site_id_fkey`,
			`ALTER TABLE alert_rule_targets
			 ADD CONSTRAINT alert_rule_targets_site_id_fkey
			 FOREIGN KEY (site_id) REFERENCES groups(id) ON DELETE SET NULL`,
			`CREATE INDEX IF NOT EXISTS idx_alert_rule_targets_site ON alert_rule_targets(site_id) WHERE site_id IS NOT NULL`,
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_alert_rule_targets_rule_site_unique ON alert_rule_targets(rule_id, site_id) WHERE site_id IS NOT NULL`,
			`ALTER TABLE incidents DROP CONSTRAINT IF EXISTS incidents_site_id_fkey`,
			`ALTER TABLE incidents
			 ADD CONSTRAINT incidents_site_id_fkey
			 FOREIGN KEY (site_id) REFERENCES groups(id) ON DELETE SET NULL`,
		},
	}
}

func schemaMigrationGroupIdForeignKeyRetargeting() schemaMigration {
	return schemaMigration{
		Version: 51,
		Name:    "group_id_foreign_key_retargeting",
		Statements: []string{
			`ALTER TABLE assets DROP CONSTRAINT IF EXISTS assets_site_id_fkey`,
			`ALTER TABLE assets DROP CONSTRAINT IF EXISTS assets_group_id_fkey`,
			`ALTER TABLE assets
			 ADD CONSTRAINT assets_group_id_fkey
			 FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE SET NULL`,

			`ALTER TABLE site_maintenance_windows DROP CONSTRAINT IF EXISTS site_maintenance_windows_site_id_fkey`,
			`ALTER TABLE site_maintenance_windows
			 ADD CONSTRAINT site_maintenance_windows_site_id_fkey
			 FOREIGN KEY (site_id) REFERENCES groups(id) ON DELETE CASCADE`,

			`ALTER TABLE site_reliability_history DROP CONSTRAINT IF EXISTS site_reliability_history_site_id_fkey`,
			`ALTER TABLE site_reliability_history
			 ADD CONSTRAINT site_reliability_history_site_id_fkey
			 FOREIGN KEY (site_id) REFERENCES groups(id) ON DELETE CASCADE`,

			`ALTER TABLE site_profile_assignments DROP CONSTRAINT IF EXISTS site_profile_assignments_site_id_fkey`,
			`ALTER TABLE site_profile_assignments DROP CONSTRAINT IF EXISTS site_profile_assignments_profile_id_fkey`,
			`ALTER TABLE site_profile_assignments
			 ADD CONSTRAINT site_profile_assignments_site_id_fkey
			 FOREIGN KEY (site_id) REFERENCES groups(id) ON DELETE CASCADE`,
			`ALTER TABLE site_profile_assignments
			 ADD CONSTRAINT site_profile_assignments_profile_id_fkey
			 FOREIGN KEY (profile_id) REFERENCES site_profiles(id) ON DELETE CASCADE`,

			`ALTER TABLE site_profile_drift_checks DROP CONSTRAINT IF EXISTS site_profile_drift_checks_site_id_fkey`,
			`ALTER TABLE site_profile_drift_checks DROP CONSTRAINT IF EXISTS site_profile_drift_checks_profile_id_fkey`,
			`ALTER TABLE site_profile_drift_checks
			 ADD CONSTRAINT site_profile_drift_checks_site_id_fkey
			 FOREIGN KEY (site_id) REFERENCES groups(id) ON DELETE CASCADE`,
			`ALTER TABLE site_profile_drift_checks
			 ADD CONSTRAINT site_profile_drift_checks_profile_id_fkey
			 FOREIGN KEY (profile_id) REFERENCES site_profiles(id) ON DELETE CASCADE`,

			`ALTER TABLE site_failover_pairs DROP CONSTRAINT IF EXISTS site_failover_pairs_primary_site_id_fkey`,
			`ALTER TABLE site_failover_pairs DROP CONSTRAINT IF EXISTS site_failover_pairs_backup_site_id_fkey`,
			`ALTER TABLE site_failover_pairs
			 ADD CONSTRAINT site_failover_pairs_primary_site_id_fkey
			 FOREIGN KEY (primary_site_id) REFERENCES groups(id) ON DELETE CASCADE`,
			`ALTER TABLE site_failover_pairs
			 ADD CONSTRAINT site_failover_pairs_backup_site_id_fkey
			 FOREIGN KEY (backup_site_id) REFERENCES groups(id) ON DELETE CASCADE`,
		},
	}
}

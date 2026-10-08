package persistence

// Schema changes owned by the group columns domain.

func schemaMigrationGroupColumnsCanonicalization() schemaMigration {
	return schemaMigration{
		Version: 52,
		Name:    "group_columns_canonicalization",
		Statements: []string{
			`ALTER TABLE site_maintenance_windows DROP CONSTRAINT IF EXISTS site_maintenance_windows_site_id_fkey`,
			`DO $$
			BEGIN
				IF EXISTS (
					SELECT 1 FROM information_schema.columns
					WHERE table_schema = current_schema()
					  AND table_name = 'site_maintenance_windows'
					  AND column_name = 'site_id'
				) AND NOT EXISTS (
					SELECT 1 FROM information_schema.columns
					WHERE table_schema = current_schema()
					  AND table_name = 'site_maintenance_windows'
					  AND column_name = 'group_id'
				) THEN
					EXECUTE 'ALTER TABLE site_maintenance_windows RENAME COLUMN site_id TO group_id';
				END IF;
			END $$`,
			`DROP INDEX IF EXISTS idx_site_maintenance_windows_site_start`,
			`DROP INDEX IF EXISTS idx_site_maintenance_windows_site_active`,
			`CREATE INDEX IF NOT EXISTS idx_site_maintenance_windows_group_start ON site_maintenance_windows(group_id, start_at DESC)`,
			`CREATE INDEX IF NOT EXISTS idx_site_maintenance_windows_group_active ON site_maintenance_windows(group_id, start_at, end_at)`,
			`ALTER TABLE site_maintenance_windows DROP CONSTRAINT IF EXISTS site_maintenance_windows_group_id_fkey`,
			`ALTER TABLE site_maintenance_windows
			 ADD CONSTRAINT site_maintenance_windows_group_id_fkey
			 FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE CASCADE`,

			`ALTER TABLE site_reliability_history DROP CONSTRAINT IF EXISTS site_reliability_history_site_id_fkey`,
			`DO $$
			BEGIN
				IF EXISTS (
					SELECT 1 FROM information_schema.columns
					WHERE table_schema = current_schema()
					  AND table_name = 'site_reliability_history'
					  AND column_name = 'site_id'
				) AND NOT EXISTS (
					SELECT 1 FROM information_schema.columns
					WHERE table_schema = current_schema()
					  AND table_name = 'site_reliability_history'
					  AND column_name = 'group_id'
				) THEN
					EXECUTE 'ALTER TABLE site_reliability_history RENAME COLUMN site_id TO group_id';
				END IF;
			END $$`,
			`DROP INDEX IF EXISTS idx_rel_hist_site_time`,
			`CREATE INDEX IF NOT EXISTS idx_rel_hist_group_time ON site_reliability_history(group_id, computed_at DESC)`,
			`ALTER TABLE site_reliability_history DROP CONSTRAINT IF EXISTS site_reliability_history_group_id_fkey`,
			`ALTER TABLE site_reliability_history
			 ADD CONSTRAINT site_reliability_history_group_id_fkey
			 FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE CASCADE`,

			`ALTER TABLE site_profile_assignments DROP CONSTRAINT IF EXISTS site_profile_assignments_site_id_fkey`,
			`DO $$
			BEGIN
				IF EXISTS (
					SELECT 1 FROM information_schema.columns
					WHERE table_schema = current_schema()
					  AND table_name = 'site_profile_assignments'
					  AND column_name = 'site_id'
				) AND NOT EXISTS (
					SELECT 1 FROM information_schema.columns
					WHERE table_schema = current_schema()
					  AND table_name = 'site_profile_assignments'
					  AND column_name = 'group_id'
				) THEN
					EXECUTE 'ALTER TABLE site_profile_assignments RENAME COLUMN site_id TO group_id';
				END IF;
			END $$`,
			`ALTER TABLE site_profile_assignments DROP CONSTRAINT IF EXISTS site_profile_assignments_group_id_fkey`,
			`ALTER TABLE site_profile_assignments
			 ADD CONSTRAINT site_profile_assignments_group_id_fkey
			 FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE CASCADE`,

			`ALTER TABLE site_profile_drift_checks DROP CONSTRAINT IF EXISTS site_profile_drift_checks_site_id_fkey`,
			`DO $$
			BEGIN
				IF EXISTS (
					SELECT 1 FROM information_schema.columns
					WHERE table_schema = current_schema()
					  AND table_name = 'site_profile_drift_checks'
					  AND column_name = 'site_id'
				) AND NOT EXISTS (
					SELECT 1 FROM information_schema.columns
					WHERE table_schema = current_schema()
					  AND table_name = 'site_profile_drift_checks'
					  AND column_name = 'group_id'
				) THEN
					EXECUTE 'ALTER TABLE site_profile_drift_checks RENAME COLUMN site_id TO group_id';
				END IF;
			END $$`,
			`DROP INDEX IF EXISTS idx_drift_site_time`,
			`CREATE INDEX IF NOT EXISTS idx_drift_group_time ON site_profile_drift_checks(group_id, checked_at DESC)`,
			`ALTER TABLE site_profile_drift_checks DROP CONSTRAINT IF EXISTS site_profile_drift_checks_group_id_fkey`,
			`ALTER TABLE site_profile_drift_checks
			 ADD CONSTRAINT site_profile_drift_checks_group_id_fkey
			 FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE CASCADE`,

			`ALTER TABLE site_failover_pairs DROP CONSTRAINT IF EXISTS site_failover_pairs_primary_site_id_fkey`,
			`ALTER TABLE site_failover_pairs DROP CONSTRAINT IF EXISTS site_failover_pairs_backup_site_id_fkey`,
			`DO $$
			BEGIN
				IF EXISTS (
					SELECT 1 FROM information_schema.columns
					WHERE table_schema = current_schema()
					  AND table_name = 'site_failover_pairs'
					  AND column_name = 'primary_site_id'
				) AND NOT EXISTS (
					SELECT 1 FROM information_schema.columns
					WHERE table_schema = current_schema()
					  AND table_name = 'site_failover_pairs'
					  AND column_name = 'primary_group_id'
				) THEN
					EXECUTE 'ALTER TABLE site_failover_pairs RENAME COLUMN primary_site_id TO primary_group_id';
				END IF;
				IF EXISTS (
					SELECT 1 FROM information_schema.columns
					WHERE table_schema = current_schema()
					  AND table_name = 'site_failover_pairs'
					  AND column_name = 'backup_site_id'
				) AND NOT EXISTS (
					SELECT 1 FROM information_schema.columns
					WHERE table_schema = current_schema()
					  AND table_name = 'site_failover_pairs'
					  AND column_name = 'backup_group_id'
				) THEN
					EXECUTE 'ALTER TABLE site_failover_pairs RENAME COLUMN backup_site_id TO backup_group_id';
				END IF;
			END $$`,
			`ALTER TABLE site_failover_pairs DROP CONSTRAINT IF EXISTS site_failover_pairs_primary_group_id_fkey`,
			`ALTER TABLE site_failover_pairs DROP CONSTRAINT IF EXISTS site_failover_pairs_backup_group_id_fkey`,
			`ALTER TABLE site_failover_pairs
			 ADD CONSTRAINT site_failover_pairs_primary_group_id_fkey
			 FOREIGN KEY (primary_group_id) REFERENCES groups(id) ON DELETE CASCADE`,
			`ALTER TABLE site_failover_pairs
			 ADD CONSTRAINT site_failover_pairs_backup_group_id_fkey
			 FOREIGN KEY (backup_group_id) REFERENCES groups(id) ON DELETE CASCADE`,

			`ALTER TABLE alert_rule_targets DROP CONSTRAINT IF EXISTS alert_rule_targets_site_id_fkey`,
			`DO $$
			BEGIN
				IF EXISTS (
					SELECT 1 FROM information_schema.columns
					WHERE table_schema = current_schema()
					  AND table_name = 'alert_rule_targets'
					  AND column_name = 'site_id'
				) AND NOT EXISTS (
					SELECT 1 FROM information_schema.columns
					WHERE table_schema = current_schema()
					  AND table_name = 'alert_rule_targets'
					  AND column_name = 'group_id'
				) THEN
					EXECUTE 'ALTER TABLE alert_rule_targets RENAME COLUMN site_id TO group_id';
				END IF;
			END $$`,
			`UPDATE alert_rule_targets art
			 SET group_id = NULL
			 WHERE group_id IS NOT NULL AND NOT EXISTS (
			 	SELECT 1 FROM groups g WHERE g.id = art.group_id
			 )`,
			`DROP INDEX IF EXISTS idx_alert_rule_targets_site`,
			`DROP INDEX IF EXISTS idx_alert_rule_targets_rule_site_unique`,
			`CREATE INDEX IF NOT EXISTS idx_alert_rule_targets_group ON alert_rule_targets(group_id) WHERE group_id IS NOT NULL`,
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_alert_rule_targets_rule_group_unique ON alert_rule_targets(rule_id, group_id) WHERE group_id IS NOT NULL`,
			`ALTER TABLE alert_rule_targets DROP CONSTRAINT IF EXISTS alert_rule_targets_group_id_fkey`,
			`ALTER TABLE alert_rule_targets
			 ADD CONSTRAINT alert_rule_targets_group_id_fkey
			 FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE SET NULL`,

			`ALTER TABLE incidents DROP CONSTRAINT IF EXISTS incidents_site_id_fkey`,
			`DO $$
			BEGIN
				IF EXISTS (
					SELECT 1 FROM information_schema.columns
					WHERE table_schema = current_schema()
					  AND table_name = 'incidents'
					  AND column_name = 'site_id'
				) AND NOT EXISTS (
					SELECT 1 FROM information_schema.columns
					WHERE table_schema = current_schema()
					  AND table_name = 'incidents'
					  AND column_name = 'group_id'
				) THEN
					EXECUTE 'ALTER TABLE incidents RENAME COLUMN site_id TO group_id';
				END IF;
			END $$`,
			`UPDATE incidents inc
			 SET group_id = NULL
			 WHERE group_id IS NOT NULL AND NOT EXISTS (
			 	SELECT 1 FROM groups g WHERE g.id = inc.group_id
			 )`,
			`DROP INDEX IF EXISTS idx_incidents_site_status`,
			`CREATE INDEX IF NOT EXISTS idx_incidents_group_status ON incidents(group_id, status, updated_at DESC)`,
			`ALTER TABLE incidents DROP CONSTRAINT IF EXISTS incidents_group_id_fkey`,
			`ALTER TABLE incidents
			 ADD CONSTRAINT incidents_group_id_fkey
			 FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE SET NULL`,
		},
	}
}

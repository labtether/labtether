package persistence

// Schema changes owned by the group filters domain.

func schemaMigrationGroupTableAndFilterCanonicalization() schemaMigration {
	return schemaMigration{
		Version: 53,
		Name:    "group_table_and_filter_canonicalization",
		Statements: []string{
			`DO $$
			BEGIN
				IF EXISTS (
					SELECT 1 FROM information_schema.columns
					WHERE table_schema = current_schema()
					  AND table_name = 'alert_routes'
					  AND column_name = 'site_filter'
				) AND NOT EXISTS (
					SELECT 1 FROM information_schema.columns
					WHERE table_schema = current_schema()
					  AND table_name = 'alert_routes'
					  AND column_name = 'group_filter'
				) THEN
					EXECUTE 'ALTER TABLE alert_routes RENAME COLUMN site_filter TO group_filter';
				END IF;
			END $$`,

			`DO $$
			BEGIN
				IF EXISTS (
					SELECT 1 FROM information_schema.tables
					WHERE table_schema = current_schema()
					  AND table_name = 'site_maintenance_windows'
				) AND NOT EXISTS (
					SELECT 1 FROM information_schema.tables
					WHERE table_schema = current_schema()
					  AND table_name = 'group_maintenance_windows'
				) THEN
					EXECUTE 'ALTER TABLE site_maintenance_windows RENAME TO group_maintenance_windows';
				END IF;
			END $$`,
			`ALTER INDEX IF EXISTS idx_site_maintenance_windows_group_start RENAME TO idx_group_maintenance_windows_group_start`,
			`ALTER INDEX IF EXISTS idx_site_maintenance_windows_group_active RENAME TO idx_group_maintenance_windows_group_active`,

			`DO $$
			BEGIN
				IF EXISTS (
					SELECT 1 FROM information_schema.tables
					WHERE table_schema = current_schema()
					  AND table_name = 'site_reliability_history'
				) AND NOT EXISTS (
					SELECT 1 FROM information_schema.tables
					WHERE table_schema = current_schema()
					  AND table_name = 'group_reliability_history'
				) THEN
					EXECUTE 'ALTER TABLE site_reliability_history RENAME TO group_reliability_history';
				END IF;
			END $$`,
			`ALTER INDEX IF EXISTS idx_rel_hist_group_time RENAME TO idx_group_reliability_history_group_time`,

			`DO $$
			BEGIN
				IF EXISTS (
					SELECT 1 FROM information_schema.tables
					WHERE table_schema = current_schema()
					  AND table_name = 'site_profiles'
				) AND NOT EXISTS (
					SELECT 1 FROM information_schema.tables
					WHERE table_schema = current_schema()
					  AND table_name = 'group_profiles'
				) THEN
					EXECUTE 'ALTER TABLE site_profiles RENAME TO group_profiles';
				END IF;
			END $$`,

			`DO $$
			BEGIN
				IF EXISTS (
					SELECT 1 FROM information_schema.tables
					WHERE table_schema = current_schema()
					  AND table_name = 'site_profile_assignments'
				) AND NOT EXISTS (
					SELECT 1 FROM information_schema.tables
					WHERE table_schema = current_schema()
					  AND table_name = 'group_profile_assignments'
				) THEN
					EXECUTE 'ALTER TABLE site_profile_assignments RENAME TO group_profile_assignments';
				END IF;
			END $$`,

			`DO $$
			BEGIN
				IF EXISTS (
					SELECT 1 FROM information_schema.tables
					WHERE table_schema = current_schema()
					  AND table_name = 'site_profile_drift_checks'
				) AND NOT EXISTS (
					SELECT 1 FROM information_schema.tables
					WHERE table_schema = current_schema()
					  AND table_name = 'group_profile_drift_checks'
				) THEN
					EXECUTE 'ALTER TABLE site_profile_drift_checks RENAME TO group_profile_drift_checks';
				END IF;
			END $$`,
			`ALTER INDEX IF EXISTS idx_drift_group_time RENAME TO idx_group_profile_drift_checks_group_time`,

			`DO $$
			BEGIN
				IF EXISTS (
					SELECT 1 FROM information_schema.tables
					WHERE table_schema = current_schema()
					  AND table_name = 'site_failover_pairs'
				) AND NOT EXISTS (
					SELECT 1 FROM information_schema.tables
					WHERE table_schema = current_schema()
					  AND table_name = 'group_failover_pairs'
				) THEN
					EXECUTE 'ALTER TABLE site_failover_pairs RENAME TO group_failover_pairs';
				END IF;
			END $$`,
		},
	}
}

func schemaMigrationGroupTableAndFilterRenames() schemaMigration {
	return schemaMigration{
		Version: 54,
		Name:    "group_table_and_filter_renames",
		Statements: []string{
			`DO $$
			BEGIN
				IF EXISTS (
					SELECT 1 FROM information_schema.tables
					WHERE table_schema = current_schema()
					  AND table_name = 'site_maintenance_windows'
				) AND NOT EXISTS (
					SELECT 1 FROM information_schema.tables
					WHERE table_schema = current_schema()
					  AND table_name = 'group_maintenance_windows'
				) THEN
					EXECUTE 'ALTER TABLE site_maintenance_windows RENAME TO group_maintenance_windows';
				END IF;
			END $$`,
			`DO $$
			BEGIN
				IF EXISTS (
					SELECT 1 FROM information_schema.tables
					WHERE table_schema = current_schema()
					  AND table_name = 'site_reliability_history'
				) AND NOT EXISTS (
					SELECT 1 FROM information_schema.tables
					WHERE table_schema = current_schema()
					  AND table_name = 'group_reliability_history'
				) THEN
					EXECUTE 'ALTER TABLE site_reliability_history RENAME TO group_reliability_history';
				END IF;
			END $$`,
			`DO $$
			BEGIN
				IF EXISTS (
					SELECT 1 FROM information_schema.tables
					WHERE table_schema = current_schema()
					  AND table_name = 'site_profiles'
				) AND NOT EXISTS (
					SELECT 1 FROM information_schema.tables
					WHERE table_schema = current_schema()
					  AND table_name = 'group_profiles'
				) THEN
					EXECUTE 'ALTER TABLE site_profiles RENAME TO group_profiles';
				END IF;
			END $$`,
			`DO $$
			BEGIN
				IF EXISTS (
					SELECT 1 FROM information_schema.tables
					WHERE table_schema = current_schema()
					  AND table_name = 'site_profile_assignments'
				) AND NOT EXISTS (
					SELECT 1 FROM information_schema.tables
					WHERE table_schema = current_schema()
					  AND table_name = 'group_profile_assignments'
				) THEN
					EXECUTE 'ALTER TABLE site_profile_assignments RENAME TO group_profile_assignments';
				END IF;
			END $$`,
			`DO $$
			BEGIN
				IF EXISTS (
					SELECT 1 FROM information_schema.tables
					WHERE table_schema = current_schema()
					  AND table_name = 'site_profile_drift_checks'
				) AND NOT EXISTS (
					SELECT 1 FROM information_schema.tables
					WHERE table_schema = current_schema()
					  AND table_name = 'group_profile_drift_checks'
				) THEN
					EXECUTE 'ALTER TABLE site_profile_drift_checks RENAME TO group_profile_drift_checks';
				END IF;
			END $$`,
			`DO $$
			BEGIN
				IF EXISTS (
					SELECT 1 FROM information_schema.tables
					WHERE table_schema = current_schema()
					  AND table_name = 'site_failover_pairs'
				) AND NOT EXISTS (
					SELECT 1 FROM information_schema.tables
					WHERE table_schema = current_schema()
					  AND table_name = 'group_failover_pairs'
				) THEN
					EXECUTE 'ALTER TABLE site_failover_pairs RENAME TO group_failover_pairs';
				END IF;
			END $$`,
			`DO $$
			BEGIN
				IF EXISTS (
					SELECT 1 FROM information_schema.columns
					WHERE table_schema = current_schema()
					  AND table_name = 'alert_routes'
					  AND column_name = 'site_filter'
				) AND NOT EXISTS (
					SELECT 1 FROM information_schema.columns
					WHERE table_schema = current_schema()
					  AND table_name = 'alert_routes'
					  AND column_name = 'group_filter'
				) THEN
					EXECUTE 'ALTER TABLE alert_routes RENAME COLUMN site_filter TO group_filter';
				END IF;
			END $$`,
		},
	}
}

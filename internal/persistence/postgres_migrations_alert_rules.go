package persistence

// Schema changes owned by the alert rules domain.

func schemaMigrationAlertingRulesTargetsEvaluations() schemaMigration {
	return schemaMigration{
		Version: 7,
		Name:    "alerting_rules_targets_evaluations",
		Statements: []string{
			`CREATE TABLE IF NOT EXISTS alert_rules (
					id TEXT PRIMARY KEY,
					name TEXT NOT NULL,
					description TEXT NOT NULL DEFAULT '',
					status TEXT NOT NULL CHECK (status IN ('active', 'paused')),
					kind TEXT NOT NULL CHECK (kind IN ('metric_threshold', 'metric_deadman', 'heartbeat_stale', 'log_pattern', 'composite')),
					severity TEXT NOT NULL CHECK (severity IN ('critical', 'high', 'medium', 'low')),
					target_scope TEXT NOT NULL CHECK (target_scope IN ('asset', 'site', 'global')),
					cooldown_seconds INT NOT NULL DEFAULT 300 CHECK (cooldown_seconds >= 0),
					reopen_after_seconds INT NOT NULL DEFAULT 60 CHECK (reopen_after_seconds >= 0),
					evaluation_interval_seconds INT NOT NULL DEFAULT 30 CHECK (evaluation_interval_seconds > 0),
					window_seconds INT NOT NULL DEFAULT 300 CHECK (window_seconds > 0),
					condition JSONB NOT NULL,
					labels JSONB NOT NULL DEFAULT '{}'::jsonb,
					metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
					created_by TEXT NOT NULL,
					created_at TIMESTAMPTZ NOT NULL,
					updated_at TIMESTAMPTZ NOT NULL,
					last_evaluated_at TIMESTAMPTZ
				)`,
			`CREATE INDEX IF NOT EXISTS idx_alert_rules_status_updated ON alert_rules(status, updated_at DESC)`,
			`CREATE INDEX IF NOT EXISTS idx_alert_rules_kind_status ON alert_rules(kind, status)`,
			`CREATE INDEX IF NOT EXISTS idx_alert_rules_severity_status ON alert_rules(severity, status)`,
			`CREATE TABLE IF NOT EXISTS alert_rule_targets (
					id TEXT PRIMARY KEY,
					rule_id TEXT NOT NULL REFERENCES alert_rules(id) ON DELETE CASCADE,
					asset_id TEXT REFERENCES assets(id) ON DELETE CASCADE,
					site_id TEXT REFERENCES groupmaintenance(id) ON DELETE CASCADE,
					selector JSONB,
					created_at TIMESTAMPTZ NOT NULL,
					CHECK (
						(
							CASE WHEN asset_id IS NOT NULL THEN 1 ELSE 0 END +
							CASE WHEN site_id IS NOT NULL THEN 1 ELSE 0 END +
							CASE WHEN selector IS NOT NULL THEN 1 ELSE 0 END
						) = 1
					)
				)`,
			`CREATE INDEX IF NOT EXISTS idx_alert_rule_targets_rule ON alert_rule_targets(rule_id)`,
			`CREATE INDEX IF NOT EXISTS idx_alert_rule_targets_asset ON alert_rule_targets(asset_id) WHERE asset_id IS NOT NULL`,
			`CREATE INDEX IF NOT EXISTS idx_alert_rule_targets_site ON alert_rule_targets(site_id) WHERE site_id IS NOT NULL`,
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_alert_rule_targets_rule_asset_unique ON alert_rule_targets(rule_id, asset_id) WHERE asset_id IS NOT NULL`,
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_alert_rule_targets_rule_site_unique ON alert_rule_targets(rule_id, site_id) WHERE site_id IS NOT NULL`,
			`CREATE TABLE IF NOT EXISTS alert_evaluations (
					id TEXT PRIMARY KEY,
					rule_id TEXT NOT NULL REFERENCES alert_rules(id) ON DELETE CASCADE,
					status TEXT NOT NULL CHECK (status IN ('ok', 'triggered', 'suppressed', 'error')),
					evaluated_at TIMESTAMPTZ NOT NULL,
					duration_ms INT NOT NULL DEFAULT 0 CHECK (duration_ms >= 0),
					candidate_count INT NOT NULL DEFAULT 0 CHECK (candidate_count >= 0),
					triggered_count INT NOT NULL DEFAULT 0 CHECK (triggered_count >= 0),
					error TEXT NOT NULL DEFAULT '',
					details JSONB NOT NULL DEFAULT '{}'::jsonb
				)`,
			`CREATE INDEX IF NOT EXISTS idx_alert_evaluations_rule_time ON alert_evaluations(rule_id, evaluated_at DESC)`,
			`CREATE INDEX IF NOT EXISTS idx_alert_evaluations_time ON alert_evaluations(evaluated_at DESC)`,
		},
	}
}

func schemaMigrationAlertInstancesSilences() schemaMigration {
	return schemaMigration{
		Version: 11,
		Name:    "alert_instances_silences",
		Statements: []string{
			`CREATE TABLE IF NOT EXISTS alert_instances (
					id TEXT PRIMARY KEY,
					rule_id TEXT NOT NULL REFERENCES alert_rules(id) ON DELETE CASCADE,
					fingerprint TEXT NOT NULL,
					status TEXT NOT NULL CHECK (status IN ('pending', 'firing', 'acknowledged', 'resolved')),
					severity TEXT NOT NULL CHECK (severity IN ('critical', 'high', 'medium', 'low')),
					labels JSONB NOT NULL DEFAULT '{}'::jsonb,
					annotations JSONB NOT NULL DEFAULT '{}'::jsonb,
					started_at TIMESTAMPTZ NOT NULL,
					resolved_at TIMESTAMPTZ,
					last_fired_at TIMESTAMPTZ NOT NULL,
					suppressed_by TEXT,
					created_at TIMESTAMPTZ NOT NULL,
					updated_at TIMESTAMPTZ NOT NULL
				)`,
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_alert_instances_active_fingerprint
					ON alert_instances(rule_id, fingerprint)
					WHERE status IN ('pending', 'firing', 'acknowledged')`,
			`CREATE INDEX IF NOT EXISTS idx_alert_instances_rule_status ON alert_instances(rule_id, status)`,
			`CREATE INDEX IF NOT EXISTS idx_alert_instances_status_updated ON alert_instances(status, updated_at DESC)`,
			`CREATE INDEX IF NOT EXISTS idx_alert_instances_severity_status ON alert_instances(severity, status)`,
			`CREATE TABLE IF NOT EXISTS alert_silences (
					id TEXT PRIMARY KEY,
					matchers JSONB NOT NULL,
					reason TEXT NOT NULL DEFAULT '',
					created_by TEXT NOT NULL,
					starts_at TIMESTAMPTZ NOT NULL,
					ends_at TIMESTAMPTZ NOT NULL,
					created_at TIMESTAMPTZ NOT NULL,
					CHECK (ends_at > starts_at)
				)`,
			`CREATE INDEX IF NOT EXISTS idx_alert_silences_active ON alert_silences(starts_at, ends_at)`,
		},
	}
}

func schemaMigrationSyntheticChecksReliabilityHistoryMaintenanceOverrides() schemaMigration {
	return schemaMigration{
		Version: 14,
		Name:    "synthetic_checks_reliability_history_maintenance_overrides",
		Statements: []string{
			`CREATE TABLE IF NOT EXISTS synthetic_checks (
				id TEXT PRIMARY KEY,
				name TEXT NOT NULL,
				check_type TEXT NOT NULL CHECK (check_type IN ('http','tcp','dns','tls_cert')),
				target TEXT NOT NULL,
				config JSONB NOT NULL DEFAULT '{}',
				interval_seconds INT NOT NULL DEFAULT 60,
				enabled BOOLEAN NOT NULL DEFAULT true,
				last_run_at TIMESTAMPTZ,
				last_status TEXT DEFAULT '',
				created_at TIMESTAMPTZ DEFAULT now(),
				updated_at TIMESTAMPTZ DEFAULT now()
			)`,
			`CREATE TABLE IF NOT EXISTS synthetic_check_results (
				id TEXT PRIMARY KEY,
				check_id TEXT NOT NULL REFERENCES synthetic_checks(id) ON DELETE CASCADE,
				status TEXT NOT NULL CHECK (status IN ('ok','fail','timeout')),
				latency_ms INT,
				error TEXT DEFAULT '',
				metadata JSONB DEFAULT '{}',
				checked_at TIMESTAMPTZ NOT NULL DEFAULT now()
			)`,
			`CREATE INDEX IF NOT EXISTS idx_synth_results_check_time ON synthetic_check_results(check_id, checked_at DESC)`,
			`CREATE TABLE IF NOT EXISTS site_reliability_history (
				id TEXT PRIMARY KEY,
				site_id TEXT NOT NULL REFERENCES groupmaintenance(id) ON DELETE CASCADE,
				score INT NOT NULL,
				grade TEXT NOT NULL,
				factors JSONB NOT NULL DEFAULT '{}',
				window_hours INT NOT NULL DEFAULT 24,
				computed_at TIMESTAMPTZ NOT NULL DEFAULT now()
			)`,
			`CREATE INDEX IF NOT EXISTS idx_rel_hist_site_time ON site_reliability_history(site_id, computed_at DESC)`,
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
		},
	}
}

func schemaMigrationAddSyntheticCheckServiceLink() schemaMigration {
	return schemaMigration{
		Version: 70,
		Name:    "add_synthetic_check_service_link",
		Statements: []string{
			`ALTER TABLE synthetic_checks ADD COLUMN IF NOT EXISTS service_id TEXT`,
			`CREATE INDEX IF NOT EXISTS idx_synthetic_checks_service_id ON synthetic_checks(service_id) WHERE service_id IS NOT NULL`,
		},
	}
}

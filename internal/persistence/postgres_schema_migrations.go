package persistence

func postgresSchemaMigrations() []schemaMigration {
	migrations := []schemaMigration{
		{
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
		},
		{
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
		},
		{
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
		},
		{
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
		},
		{
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
		},
		{
			Version: 6,
			Name:    "terminal_credentials_and_asset_terminal_config",
			Statements: []string{
				`CREATE TABLE IF NOT EXISTS credential_profiles (
					id TEXT PRIMARY KEY,
					name TEXT NOT NULL,
					kind TEXT NOT NULL,
					username TEXT,
					description TEXT,
					status TEXT NOT NULL,
					metadata JSONB,
					secret_ciphertext TEXT NOT NULL,
					passphrase_ciphertext TEXT,
					created_at TIMESTAMPTZ NOT NULL,
					updated_at TIMESTAMPTZ NOT NULL,
					rotated_at TIMESTAMPTZ,
					last_used_at TIMESTAMPTZ,
					expires_at TIMESTAMPTZ
				)`,
				`CREATE INDEX IF NOT EXISTS idx_credential_profiles_updated ON credential_profiles(updated_at DESC)`,
				`CREATE INDEX IF NOT EXISTS idx_credential_profiles_status ON credential_profiles(status, updated_at DESC)`,
				`CREATE TABLE IF NOT EXISTS asset_terminal_configs (
					asset_id TEXT PRIMARY KEY REFERENCES assets(id) ON DELETE CASCADE,
					host TEXT NOT NULL,
					port INT NOT NULL DEFAULT 22,
					username TEXT,
					strict_host_key BOOLEAN NOT NULL DEFAULT TRUE,
					host_key TEXT,
					credential_profile_id TEXT REFERENCES credential_profiles(id) ON DELETE SET NULL,
					updated_at TIMESTAMPTZ NOT NULL
				)`,
				`CREATE INDEX IF NOT EXISTS idx_asset_terminal_configs_credential ON asset_terminal_configs(credential_profile_id)`,
			},
		},
		{
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
		},
		{
			Version: 8,
			Name:    "incidents_and_alert_links",
			Statements: []string{
				`CREATE TABLE IF NOT EXISTS incidents (
					id TEXT PRIMARY KEY,
					title TEXT NOT NULL,
					summary TEXT NOT NULL DEFAULT '',
					status TEXT NOT NULL CHECK (status IN ('open', 'investigating', 'mitigated', 'resolved', 'closed')),
					severity TEXT NOT NULL CHECK (severity IN ('critical', 'high', 'medium', 'low')),
					source TEXT NOT NULL CHECK (source IN ('manual', 'alert_auto')),
					site_id TEXT REFERENCES groupmaintenance(id) ON DELETE SET NULL,
					primary_asset_id TEXT REFERENCES assets(id) ON DELETE SET NULL,
					assignee TEXT,
					created_by TEXT NOT NULL,
					opened_at TIMESTAMPTZ NOT NULL,
					mitigated_at TIMESTAMPTZ,
					resolved_at TIMESTAMPTZ,
					closed_at TIMESTAMPTZ,
					metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
					created_at TIMESTAMPTZ NOT NULL,
					updated_at TIMESTAMPTZ NOT NULL
				)`,
				`CREATE INDEX IF NOT EXISTS idx_incidents_status_updated ON incidents(status, updated_at DESC)`,
				`CREATE INDEX IF NOT EXISTS idx_incidents_severity_status ON incidents(severity, status, updated_at DESC)`,
				`CREATE INDEX IF NOT EXISTS idx_incidents_site_status ON incidents(site_id, status, updated_at DESC)`,
				`CREATE INDEX IF NOT EXISTS idx_incidents_assignee_status ON incidents(assignee, status, updated_at DESC)`,
				`CREATE TABLE IF NOT EXISTS incident_alert_links (
					id TEXT PRIMARY KEY,
					incident_id TEXT NOT NULL REFERENCES incidents(id) ON DELETE CASCADE,
					alert_rule_id TEXT REFERENCES alert_rules(id) ON DELETE SET NULL,
					alert_instance_id TEXT,
					alert_fingerprint TEXT,
					link_type TEXT NOT NULL CHECK (link_type IN ('trigger', 'related', 'symptom', 'cause')),
					created_by TEXT NOT NULL,
					created_at TIMESTAMPTZ NOT NULL,
					CHECK (
						alert_rule_id IS NOT NULL OR alert_instance_id IS NOT NULL OR alert_fingerprint IS NOT NULL
					)
				)`,
				`CREATE INDEX IF NOT EXISTS idx_incident_alert_links_incident ON incident_alert_links(incident_id, created_at DESC)`,
				`CREATE INDEX IF NOT EXISTS idx_incident_alert_links_rule ON incident_alert_links(alert_rule_id) WHERE alert_rule_id IS NOT NULL`,
				`CREATE INDEX IF NOT EXISTS idx_incident_alert_links_instance ON incident_alert_links(alert_instance_id) WHERE alert_instance_id IS NOT NULL`,
				`CREATE UNIQUE INDEX IF NOT EXISTS idx_incident_alert_links_incident_rule_unique ON incident_alert_links(incident_id, alert_rule_id) WHERE alert_rule_id IS NOT NULL`,
				`CREATE UNIQUE INDEX IF NOT EXISTS idx_incident_alert_links_incident_instance_unique ON incident_alert_links(incident_id, alert_instance_id) WHERE alert_instance_id IS NOT NULL`,
			},
		},
		{
			Version: 9,
			Name:    "job_queue",
			Statements: []string{
				`CREATE TABLE IF NOT EXISTS job_queue (
					id TEXT PRIMARY KEY,
					kind TEXT NOT NULL,
					status TEXT NOT NULL DEFAULT 'queued',
					payload JSONB NOT NULL,
					attempts INT NOT NULL DEFAULT 0,
					max_attempts INT NOT NULL DEFAULT 5,
					error TEXT NOT NULL DEFAULT '',
					created_at TIMESTAMPTZ NOT NULL,
					updated_at TIMESTAMPTZ NOT NULL,
					locked_at TIMESTAMPTZ,
					completed_at TIMESTAMPTZ
				)`,
				`CREATE INDEX IF NOT EXISTS idx_job_queue_status_created ON job_queue(status, created_at ASC)`,
				`CREATE INDEX IF NOT EXISTS idx_job_queue_kind_status ON job_queue(kind, status)`,
				`CREATE INDEX IF NOT EXISTS idx_job_queue_dead_lettered ON job_queue(created_at DESC) WHERE status = 'dead_lettered'`,
			},
		},
		{
			Version: 10,
			Name:    "auth_users_sessions",
			Statements: []string{
				`CREATE TABLE IF NOT EXISTS users (
					id TEXT PRIMARY KEY,
					username TEXT NOT NULL UNIQUE,
					password_hash TEXT NOT NULL,
					created_at TIMESTAMPTZ NOT NULL,
					updated_at TIMESTAMPTZ NOT NULL
				)`,
				`CREATE TABLE IF NOT EXISTS sessions (
					id TEXT PRIMARY KEY,
					user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
					token_hash TEXT NOT NULL,
					expires_at TIMESTAMPTZ NOT NULL,
					created_at TIMESTAMPTZ NOT NULL
				)`,
				`CREATE INDEX IF NOT EXISTS idx_sessions_token_hash ON sessions(token_hash)`,
				`CREATE INDEX IF NOT EXISTS idx_sessions_user_expires ON sessions(user_id, expires_at DESC)`,
				`CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at)`,
			},
		},
		{
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
		},
		{
			Version: 12,
			Name:    "notification_channels_routes_history",
			Statements: []string{
				`CREATE TABLE IF NOT EXISTS notification_channels (
					id TEXT PRIMARY KEY,
					name TEXT NOT NULL,
					type TEXT NOT NULL,
					config JSONB NOT NULL DEFAULT '{}'::jsonb,
					enabled BOOLEAN NOT NULL DEFAULT TRUE,
					created_at TIMESTAMPTZ NOT NULL,
					updated_at TIMESTAMPTZ NOT NULL
				)`,
				`CREATE INDEX IF NOT EXISTS idx_notification_channels_type ON notification_channels(type)`,
				`CREATE TABLE IF NOT EXISTS alert_routes (
					id TEXT PRIMARY KEY,
					name TEXT NOT NULL,
					matchers JSONB NOT NULL DEFAULT '{}'::jsonb,
					channel_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
					severity_filter TEXT,
					site_filter TEXT,
					group_by JSONB NOT NULL DEFAULT '[]'::jsonb,
					group_wait_seconds INT NOT NULL DEFAULT 30,
					group_interval_seconds INT NOT NULL DEFAULT 300,
					repeat_interval_seconds INT NOT NULL DEFAULT 3600,
					enabled BOOLEAN NOT NULL DEFAULT TRUE,
					created_at TIMESTAMPTZ NOT NULL,
					updated_at TIMESTAMPTZ NOT NULL
				)`,
				`CREATE INDEX IF NOT EXISTS idx_alert_routes_enabled ON alert_routes(enabled, updated_at DESC)`,
				`CREATE TABLE IF NOT EXISTS notification_history (
					id TEXT PRIMARY KEY,
					channel_id TEXT NOT NULL REFERENCES notification_channels(id) ON DELETE CASCADE,
					alert_instance_id TEXT REFERENCES alert_instances(id) ON DELETE SET NULL,
					route_id TEXT REFERENCES alert_routes(id) ON DELETE SET NULL,
					status TEXT NOT NULL CHECK (status IN ('pending', 'sent', 'failed')),
					sent_at TIMESTAMPTZ,
					error TEXT NOT NULL DEFAULT '',
					created_at TIMESTAMPTZ NOT NULL
				)`,
				`CREATE INDEX IF NOT EXISTS idx_notification_history_channel ON notification_history(channel_id, created_at DESC)`,
				`CREATE INDEX IF NOT EXISTS idx_notification_history_status ON notification_history(status, created_at DESC)`,
			},
		},
		{
			Version: 13,
			Name:    "asset_dependencies_incident_assets",
			Statements: []string{
				`CREATE TABLE IF NOT EXISTS asset_dependencies (
					id TEXT PRIMARY KEY,
					source_asset_id TEXT NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
					target_asset_id TEXT NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
					relationship_type TEXT NOT NULL CHECK (relationship_type IN ('runs_on', 'depends_on', 'provides_to', 'connected_to')),
					direction TEXT NOT NULL CHECK (direction IN ('upstream', 'downstream', 'bidirectional')),
					criticality TEXT NOT NULL CHECK (criticality IN ('critical', 'high', 'medium', 'low')),
					metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
					created_at TIMESTAMPTZ NOT NULL,
					updated_at TIMESTAMPTZ NOT NULL,
					CHECK (source_asset_id != target_asset_id)
				)`,
				`CREATE UNIQUE INDEX IF NOT EXISTS idx_asset_dependencies_unique ON asset_dependencies(source_asset_id, target_asset_id, relationship_type)`,
				`CREATE INDEX IF NOT EXISTS idx_asset_dependencies_source ON asset_dependencies(source_asset_id)`,
				`CREATE INDEX IF NOT EXISTS idx_asset_dependencies_target ON asset_dependencies(target_asset_id)`,
				`CREATE TABLE IF NOT EXISTS incident_assets (
					id TEXT PRIMARY KEY,
					incident_id TEXT NOT NULL REFERENCES incidents(id) ON DELETE CASCADE,
					asset_id TEXT NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
					role TEXT NOT NULL CHECK (role IN ('primary', 'impacted', 'related', 'contributing')),
					created_at TIMESTAMPTZ NOT NULL
				)`,
				`CREATE UNIQUE INDEX IF NOT EXISTS idx_incident_assets_unique ON incident_assets(incident_id, asset_id)`,
				`CREATE INDEX IF NOT EXISTS idx_incident_assets_incident ON incident_assets(incident_id, created_at DESC)`,
				`CREATE INDEX IF NOT EXISTS idx_incident_assets_asset ON incident_assets(asset_id)`,
			},
		},
	}
	migrations = append(migrations, schemaMigration{
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
	})
	migrations = append(migrations, schemaMigration{
		Version: 15,
		Name:    "incident_events",
		Statements: []string{
			`CREATE TABLE IF NOT EXISTS incident_events (
				id TEXT PRIMARY KEY,
				incident_id TEXT NOT NULL REFERENCES incidents(id) ON DELETE CASCADE,
				event_type TEXT NOT NULL CHECK (event_type IN ('metric_anomaly','log_burst','action_run','update_run','alert_fired','alert_resolved','config_change','audit','heartbeat_change')),
				source_ref TEXT NOT NULL DEFAULT '',
				summary TEXT NOT NULL DEFAULT '',
				severity TEXT DEFAULT 'info',
				metadata JSONB DEFAULT '{}',
				occurred_at TIMESTAMPTZ NOT NULL,
				created_at TIMESTAMPTZ DEFAULT now(),
				UNIQUE(incident_id, source_ref)
			)`,
			`CREATE INDEX IF NOT EXISTS idx_inc_events_incident_time ON incident_events(incident_id, occurred_at DESC)`,
		},
	})
	migrations = append(migrations, schemaMigration{
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
	})
	migrations = append(migrations, schemaMigration{
		Version: 17,
		Name:    "hub_collectors",
		Statements: []string{
			`CREATE TABLE IF NOT EXISTS hub_collectors (
				id TEXT PRIMARY KEY,
				asset_id TEXT NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
				collector_type TEXT NOT NULL CHECK (collector_type IN ('ssh','winrm','api')),
				config JSONB NOT NULL DEFAULT '{}',
				enabled BOOLEAN NOT NULL DEFAULT true,
				interval_seconds INT NOT NULL DEFAULT 60,
				last_collected_at TIMESTAMPTZ,
				last_status TEXT DEFAULT '',
				last_error TEXT DEFAULT '',
				created_at TIMESTAMPTZ DEFAULT now(),
				updated_at TIMESTAMPTZ DEFAULT now(),
				UNIQUE(asset_id)
			)`,
		},
	})

	migrations = append(migrations, schemaMigration{
		Version: 18,
		Name:    "incident_postmortem",
		Statements: []string{
			`ALTER TABLE incidents ADD COLUMN IF NOT EXISTS root_cause TEXT DEFAULT ''`,
			`ALTER TABLE incidents ADD COLUMN IF NOT EXISTS action_items JSONB DEFAULT '[]'`,
			`ALTER TABLE incidents ADD COLUMN IF NOT EXISTS lessons_learned TEXT DEFAULT ''`,
		},
	})

	migrations = append(migrations, schemaMigration{
		Version: 19,
		Name:    "enrollment_and_agent_tokens",
		Statements: []string{
			`CREATE TABLE IF NOT EXISTS enrollment_tokens (
				id TEXT PRIMARY KEY,
				token_hash TEXT NOT NULL UNIQUE,
				label TEXT NOT NULL DEFAULT '',
				expires_at TIMESTAMPTZ NOT NULL,
				max_uses INT NOT NULL DEFAULT 0,
				use_count INT NOT NULL DEFAULT 0,
				created_at TIMESTAMPTZ NOT NULL,
				revoked_at TIMESTAMPTZ
			)`,
			`CREATE INDEX IF NOT EXISTS idx_enrollment_tokens_expires ON enrollment_tokens(expires_at)`,
			`CREATE TABLE IF NOT EXISTS agent_tokens (
				id TEXT PRIMARY KEY,
				asset_id TEXT NOT NULL,
				token_hash TEXT NOT NULL UNIQUE,
				status TEXT NOT NULL CHECK (status IN ('active', 'revoked')),
				enrolled_via TEXT,
				last_used_at TIMESTAMPTZ,
				created_at TIMESTAMPTZ NOT NULL,
				revoked_at TIMESTAMPTZ
			)`,
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_tokens_asset_active ON agent_tokens(asset_id) WHERE status = 'active'`,
			`CREATE INDEX IF NOT EXISTS idx_agent_tokens_token_hash ON agent_tokens(token_hash)`,
		},
	})

	migrations = append(migrations, schemaMigration{
		Version: 20,
		Name:    "agent_presence",
		Statements: []string{
			`CREATE TABLE IF NOT EXISTS agent_presence (
				asset_id TEXT PRIMARY KEY REFERENCES assets(id) ON DELETE CASCADE,
				transport TEXT NOT NULL DEFAULT 'agent',
				connected_at TIMESTAMPTZ NOT NULL,
				last_heartbeat_at TIMESTAMPTZ NOT NULL,
				session_id TEXT NOT NULL,
				metadata JSONB DEFAULT '{}'
			)`,
			`ALTER TABLE assets ADD COLUMN IF NOT EXISTS transport_type TEXT DEFAULT 'offline'`,
		},
	})

	migrations = append(migrations, schemaMigration{
		Version: 21,
		Name:    "asset_desktop_configs",
		Statements: []string{
			`CREATE TABLE IF NOT EXISTS asset_desktop_configs (
				asset_id TEXT PRIMARY KEY REFERENCES assets(id) ON DELETE CASCADE,
				vnc_port INTEGER NOT NULL DEFAULT 5900,
				credential_profile_id TEXT REFERENCES credential_profiles(id) ON DELETE SET NULL,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
			)`,
		},
	})

	migrations = append(migrations, schemaMigration{
		Version: 22,
		Name:    "hub_collectors_add_proxmox",
		Statements: []string{
			`ALTER TABLE hub_collectors DROP CONSTRAINT IF EXISTS hub_collectors_collector_type_check`,
			`ALTER TABLE hub_collectors
			 ADD CONSTRAINT hub_collectors_collector_type_check
			 CHECK (collector_type IN ('ssh','winrm','api','proxmox'))`,
		},
	})

	// Version 23 is a placeholder to fill the gap between 22 and 24.
	// It was skipped during development; this no-op preserves migration sequence integrity.
	migrations = append(migrations, schemaMigration{
		Version:    23,
		Name:       "noop_sequence_placeholder",
		Statements: []string{},
	})

	migrations = append(migrations, schemaMigration{
		Version: 24,
		Name:    "hub_collectors_add_truenas",
		Statements: []string{
			`ALTER TABLE hub_collectors DROP CONSTRAINT IF EXISTS hub_collectors_collector_type_check`,
			`ALTER TABLE hub_collectors
			 ADD CONSTRAINT hub_collectors_collector_type_check
			 CHECK (collector_type IN ('ssh','winrm','api','proxmox','truenas'))`,
		},
	})

	migrations = append(migrations, schemaMigration{
		Version: 25,
		Name:    "hub_collectors_add_portainer_docker",
		Statements: []string{
			`ALTER TABLE hub_collectors DROP CONSTRAINT IF EXISTS hub_collectors_collector_type_check`,
			`ALTER TABLE hub_collectors
			 ADD CONSTRAINT hub_collectors_collector_type_check
			 CHECK (collector_type IN ('ssh','winrm','api','proxmox','truenas','portainer','docker'))`,
		},
	})

	migrations = append(migrations, schemaMigration{
		Version: 26,
		Name:    "hub_collectors_add_pbs",
		Statements: []string{
			`ALTER TABLE hub_collectors DROP CONSTRAINT IF EXISTS hub_collectors_collector_type_check`,
			`ALTER TABLE hub_collectors
			 ADD CONSTRAINT hub_collectors_collector_type_check
			 CHECK (collector_type IN ('ssh','winrm','api','proxmox','pbs','truenas','portainer','docker'))`,
		},
	})

	migrations = append(migrations, schemaMigration{
		Version: 27,
		Name:    "hub_collectors_add_homeassistant",
		Statements: []string{
			`ALTER TABLE hub_collectors DROP CONSTRAINT IF EXISTS hub_collectors_collector_type_check`,
			`ALTER TABLE hub_collectors
			 ADD CONSTRAINT hub_collectors_collector_type_check
			 CHECK (collector_type IN ('ssh','winrm','api','proxmox','pbs','truenas','portainer','docker','homeassistant'))`,
		},
	})

	migrations = append(migrations, schemaMigration{
		Version: 30,
		Name:    "canonical_model_persistence",
		Statements: []string{
			`CREATE TABLE IF NOT EXISTS provider_instances (
				id TEXT PRIMARY KEY,
				kind TEXT NOT NULL CHECK (kind IN ('agent','connector')),
				provider TEXT NOT NULL,
				display_name TEXT NOT NULL,
				version TEXT,
				status TEXT NOT NULL CHECK (status IN ('healthy','degraded','offline','unknown')),
				scope TEXT NOT NULL CHECK (scope IN ('global','site')),
				config_ref TEXT,
				metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
				last_seen_at TIMESTAMPTZ NOT NULL,
				created_at TIMESTAMPTZ NOT NULL,
				updated_at TIMESTAMPTZ NOT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS idx_provider_instances_provider_updated ON provider_instances(provider, updated_at DESC)`,
			`CREATE TABLE IF NOT EXISTS resource_external_refs (
				resource_id TEXT NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
				provider_instance_id TEXT NOT NULL REFERENCES provider_instances(id) ON DELETE CASCADE,
				external_id TEXT NOT NULL,
				external_type TEXT,
				external_parent_id TEXT,
				raw_locator TEXT,
				created_at TIMESTAMPTZ NOT NULL,
				updated_at TIMESTAMPTZ NOT NULL,
				PRIMARY KEY (resource_id, provider_instance_id, external_id)
			)`,
			`CREATE INDEX IF NOT EXISTS idx_resource_external_refs_provider_external ON resource_external_refs(provider_instance_id, external_id)`,
			`CREATE INDEX IF NOT EXISTS idx_resource_external_refs_resource ON resource_external_refs(resource_id)`,
			`CREATE TABLE IF NOT EXISTS canonical_resource_relationships (
				id TEXT PRIMARY KEY,
				provider_instance_id TEXT NOT NULL REFERENCES provider_instances(id) ON DELETE CASCADE,
				source_resource_id TEXT NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
				target_resource_id TEXT NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
				relationship_type TEXT NOT NULL CHECK (relationship_type IN ('contains','runs_on','depends_on','connected_to','backs_up','replicates_to','managed_by','member_of')),
				direction TEXT NOT NULL CHECK (direction IN ('upstream','downstream','bidirectional')),
				criticality TEXT NOT NULL CHECK (criticality IN ('critical','high','medium','low')),
				inferred BOOLEAN NOT NULL DEFAULT true,
				confidence INT NOT NULL DEFAULT 0 CHECK (confidence >= 0 AND confidence <= 100),
				evidence JSONB NOT NULL DEFAULT '{}'::jsonb,
				created_at TIMESTAMPTZ NOT NULL,
				updated_at TIMESTAMPTZ NOT NULL,
				CHECK (source_resource_id <> target_resource_id)
			)`,
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_canonical_relationship_provider_unique
				ON canonical_resource_relationships(provider_instance_id, source_resource_id, target_resource_id, relationship_type)`,
			`CREATE INDEX IF NOT EXISTS idx_canonical_relationship_source ON canonical_resource_relationships(source_resource_id)`,
			`CREATE INDEX IF NOT EXISTS idx_canonical_relationship_target ON canonical_resource_relationships(target_resource_id)`,
			`CREATE INDEX IF NOT EXISTS idx_canonical_relationship_provider_updated ON canonical_resource_relationships(provider_instance_id, updated_at DESC)`,
			`CREATE TABLE IF NOT EXISTS canonical_capability_sets (
				subject_type TEXT NOT NULL CHECK (subject_type IN ('provider','resource')),
				subject_id TEXT NOT NULL,
				provider_instance_id TEXT REFERENCES provider_instances(id) ON DELETE CASCADE,
				capabilities JSONB NOT NULL DEFAULT '[]'::jsonb,
				updated_at TIMESTAMPTZ NOT NULL,
				PRIMARY KEY (subject_type, subject_id)
			)`,
			`CREATE INDEX IF NOT EXISTS idx_canonical_capability_provider_updated ON canonical_capability_sets(provider_instance_id, updated_at DESC)`,
			`CREATE TABLE IF NOT EXISTS canonical_template_bindings (
				resource_id TEXT PRIMARY KEY REFERENCES assets(id) ON DELETE CASCADE,
				template_id TEXT NOT NULL,
				tabs JSONB NOT NULL DEFAULT '[]'::jsonb,
				operations JSONB NOT NULL DEFAULT '[]'::jsonb,
				updated_at TIMESTAMPTZ NOT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS idx_canonical_template_bindings_updated ON canonical_template_bindings(updated_at DESC)`,
			`CREATE TABLE IF NOT EXISTS canonical_ingest_checkpoints (
				provider_instance_id TEXT NOT NULL REFERENCES provider_instances(id) ON DELETE CASCADE,
				stream TEXT NOT NULL,
				cursor TEXT,
				synced_at TIMESTAMPTZ NOT NULL,
				PRIMARY KEY (provider_instance_id, stream)
			)`,
			`CREATE TABLE IF NOT EXISTS canonical_reconciliation_results (
				id TEXT PRIMARY KEY,
				provider_instance_id TEXT NOT NULL REFERENCES provider_instances(id) ON DELETE CASCADE,
				created_count INT NOT NULL DEFAULT 0,
				updated_count INT NOT NULL DEFAULT 0,
				stale_count INT NOT NULL DEFAULT 0,
				error_count INT NOT NULL DEFAULT 0,
				started_at TIMESTAMPTZ NOT NULL,
				finished_at TIMESTAMPTZ NOT NULL
			)`,
			`CREATE INDEX IF NOT EXISTS idx_canonical_reconciliation_provider_finished ON canonical_reconciliation_results(provider_instance_id, finished_at DESC)`,
		},
	})

	migrations = append(migrations, schemaMigration{
		Version: 28,
		Name:    "asset_tags",
		Statements: []string{
			`ALTER TABLE assets ADD COLUMN IF NOT EXISTS tags JSONB NOT NULL DEFAULT '[]'::jsonb`,
		},
	})

	migrations = append(migrations, schemaMigration{
		Version: 29,
		Name:    "agent_tokens_expiry",
		Statements: []string{
			`ALTER TABLE agent_tokens ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ`,
			`UPDATE agent_tokens SET expires_at = created_at + INTERVAL '30 days' WHERE expires_at IS NULL`,
			`ALTER TABLE agent_tokens ALTER COLUMN expires_at SET NOT NULL`,
			`CREATE INDEX IF NOT EXISTS idx_agent_tokens_expires ON agent_tokens(expires_at)`,
		},
	})

	migrations = append(migrations, schemaMigration{
		Version: 31,
		Name:    "normalize_agent_source",
		Statements: []string{
			`UPDATE assets SET source = 'agent' WHERE source = 'labtether-agent'`,
			`UPDATE provider_instances SET provider = 'agent' WHERE provider = 'labtether-agent'`,
		},
	})

	migrations = append(migrations, schemaMigration{
		Version: 32,
		Name:    "add_hosted_on_relationship_type",
		Statements: []string{
			`ALTER TABLE asset_dependencies DROP CONSTRAINT asset_dependencies_relationship_type_check`,
			`ALTER TABLE asset_dependencies ADD CONSTRAINT asset_dependencies_relationship_type_check CHECK (relationship_type IN ('runs_on', 'hosted_on', 'depends_on', 'provides_to', 'connected_to'))`,
		},
	})

	migrations = append(migrations, schemaMigration{
		Version: 33,
		Name:    "notification_history_retry_columns",
		Statements: []string{
			`ALTER TABLE notification_history ADD COLUMN IF NOT EXISTS retry_count integer NOT NULL DEFAULT 0`,
			`ALTER TABLE notification_history ADD COLUMN IF NOT EXISTS max_retries integer NOT NULL DEFAULT 3`,
			`ALTER TABLE notification_history ADD COLUMN IF NOT EXISTS next_retry_at timestamptz`,
			`CREATE INDEX IF NOT EXISTS idx_notification_history_pending_retry ON notification_history(next_retry_at) WHERE status = 'failed'`,
		},
	})

	migrations = append(migrations, schemaMigration{
		Version: 34,
		Name:    "terminal_uplift",
		Statements: []string{
			`CREATE TABLE IF NOT EXISTS terminal_workspace_tabs (
				id          TEXT PRIMARY KEY,
				name        TEXT NOT NULL,
				layout      TEXT NOT NULL DEFAULT 'single',
				panes       JSONB NOT NULL DEFAULT '[]',
				sort_order  INT NOT NULL DEFAULT 0,
				created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
			)`,
			`CREATE TABLE IF NOT EXISTS terminal_snippets (
				id          TEXT PRIMARY KEY,
				name        TEXT NOT NULL,
				command     TEXT NOT NULL,
				description TEXT NOT NULL DEFAULT '',
				scope       TEXT NOT NULL DEFAULT 'global',
				shortcut    TEXT NOT NULL DEFAULT '',
				sort_order  INT NOT NULL DEFAULT 0,
				created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
			)`,
			`CREATE INDEX IF NOT EXISTS idx_terminal_snippets_scope ON terminal_snippets(scope)`,
			`CREATE TABLE IF NOT EXISTS terminal_preferences (
				user_id         TEXT PRIMARY KEY DEFAULT 'default',
				theme           TEXT NOT NULL DEFAULT 'labtether-dark',
				font_family     TEXT NOT NULL DEFAULT 'JetBrains Mono',
				font_size       INT NOT NULL DEFAULT 14,
				cursor_style    TEXT NOT NULL DEFAULT 'block',
				cursor_blink    BOOLEAN NOT NULL DEFAULT true,
				scrollback      INT NOT NULL DEFAULT 5000,
				toolbar_keys    JSONB,
				auto_reconnect  BOOLEAN NOT NULL DEFAULT false,
				updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
			)`,
			`CREATE TABLE IF NOT EXISTS terminal_output_buffer (
				session_id  TEXT NOT NULL,
				seq         BIGSERIAL,
				data        BYTEA NOT NULL,
				recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (session_id, seq)
			)`,
			`CREATE INDEX IF NOT EXISTS idx_terminal_output_buffer_recorded ON terminal_output_buffer(recorded_at)`,
		},
	})

	migrations = append(migrations, schemaMigration{
		Version: 35,
		Name:    "session_recordings",
		Statements: []string{
			`ALTER TABLE retention_settings ADD COLUMN IF NOT EXISTS recordings_window TEXT NOT NULL DEFAULT '30d'`,
			`CREATE TABLE IF NOT EXISTS session_recordings (
				id          TEXT PRIMARY KEY,
				session_id  TEXT NOT NULL,
				asset_id    TEXT NOT NULL,
				actor_id    TEXT NOT NULL,
				protocol    TEXT NOT NULL,
				file_path   TEXT NOT NULL,
				file_size   BIGINT NOT NULL DEFAULT 0,
				duration_ms BIGINT NOT NULL DEFAULT 0,
				status      TEXT NOT NULL DEFAULT 'recording',
				created_at  TIMESTAMPTZ NOT NULL,
				stopped_at  TIMESTAMPTZ
			)`,
			`CREATE INDEX IF NOT EXISTS idx_session_recordings_session ON session_recordings(session_id)`,
			`CREATE INDEX IF NOT EXISTS idx_session_recordings_asset_created ON session_recordings(asset_id, created_at DESC)`,
			`CREATE INDEX IF NOT EXISTS idx_session_recordings_created ON session_recordings(created_at DESC)`,
		},
	})

	migrations = append(migrations, schemaMigration{
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
	})

	migrations = append(migrations, schemaMigration{
		Version: 37,
		Name:    "push_devices",
		Statements: []string{
			`CREATE TABLE IF NOT EXISTS push_devices (
				id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
				user_id     TEXT NOT NULL,
				device_id   TEXT NOT NULL,
				platform    TEXT NOT NULL DEFAULT 'ios',
				push_token  TEXT NOT NULL,
				bundle_id   TEXT NOT NULL DEFAULT '',
				created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				UNIQUE(user_id, device_id)
			)`,
			`CREATE INDEX IF NOT EXISTS idx_push_devices_user ON push_devices(user_id)`,
			`CREATE INDEX IF NOT EXISTS idx_push_devices_token ON push_devices(push_token)`,
		},
	})

	migrations = append(migrations, schemaMigration{
		Version: 38,
		Name:    "log_events_source_level_timestamp_index",
		Statements: []string{
			`CREATE INDEX IF NOT EXISTS idx_log_events_source_level_timestamp ON log_events(source, level, timestamp DESC)`,
		},
	})

	migrations = append(migrations, schemaMigration{
		Version: 39,
		Name:    "log_events_dead_letter_error_timestamp_index",
		Statements: []string{
			`CREATE INDEX IF NOT EXISTS idx_log_events_dead_letter_error_timestamp
				ON log_events(timestamp DESC)
				WHERE source = 'dead_letter' AND level = 'error'`,
		},
	})

	migrations = append(migrations, schemaMigration{
		Version: 40,
		Name:    "log_events_timestamp_source_index",
		Statements: []string{
			`CREATE INDEX IF NOT EXISTS idx_log_events_timestamp_source ON log_events(timestamp DESC, source)`,
		},
	})

	migrations = append(migrations, schemaMigration{
		Version: 41,
		Name:    "log_events_site_projection_timestamp_index",
		Statements: []string{
			`CREATE INDEX IF NOT EXISTS idx_log_events_site_projection_timestamp
				ON log_events ((NULLIF(BTRIM(fields->>'site_id'), '')), timestamp DESC)`,
		},
	})

	migrations = append(migrations, schemaMigration{
		Version: 42,
		Name:    "auth_roles_and_oidc_identity",
		Statements: []string{
			`ALTER TABLE users ADD COLUMN IF NOT EXISTS role TEXT NOT NULL DEFAULT 'owner'`,
			`ALTER TABLE users ADD COLUMN IF NOT EXISTS auth_provider TEXT NOT NULL DEFAULT 'local'`,
			`ALTER TABLE users ADD COLUMN IF NOT EXISTS oidc_subject TEXT`,
			`UPDATE users
			 SET role = 'owner'
			 WHERE role IS NULL OR BTRIM(role) = ''`,
			`UPDATE users
			 SET auth_provider = 'local'
			 WHERE auth_provider IS NULL OR BTRIM(auth_provider) = ''`,
			`CREATE INDEX IF NOT EXISTS idx_users_role ON users(role)`,
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_users_auth_provider_subject
				ON users(auth_provider, oidc_subject)
				WHERE oidc_subject IS NOT NULL`,
		},
	})

	migrations = append(migrations, schemaMigration{
		Version: 43,
		Name:    "web_service_overrides_tags",
		Statements: []string{
			`ALTER TABLE web_service_overrides ADD COLUMN IF NOT EXISTS tags_override TEXT NOT NULL DEFAULT ''`,
		},
	})

	migrations = append(migrations, schemaMigration{
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
	})

	migrations = append(migrations, schemaMigration{
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
	})

	// --- Device hierarchy: groups table, site migration, and link suggestions ---

	migrations = append(migrations, schemaMigration{
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
	})

	migrations = append(migrations, schemaMigration{
		Version: 47,
		Name:    "terminal_persistent_sessions",
		Statements: []string{
			`ALTER TABLE terminal_sessions ADD COLUMN IF NOT EXISTS persistent_session_id TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE terminal_sessions ADD COLUMN IF NOT EXISTS tmux_session_name TEXT NOT NULL DEFAULT ''`,
			`CREATE INDEX IF NOT EXISTS idx_terminal_sessions_persistent_session_id ON terminal_sessions(persistent_session_id)`,
			`CREATE TABLE IF NOT EXISTS terminal_persistent_sessions (
				id                TEXT PRIMARY KEY,
				actor_id          TEXT NOT NULL,
				target            TEXT NOT NULL,
				title             TEXT NOT NULL,
				status            TEXT NOT NULL DEFAULT 'detached',
				tmux_session_name TEXT NOT NULL,
				created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				last_attached_at  TIMESTAMPTZ,
				last_detached_at  TIMESTAMPTZ,
				UNIQUE (actor_id, target)
			)`,
			`CREATE INDEX IF NOT EXISTS idx_terminal_persistent_sessions_actor_updated ON terminal_persistent_sessions(actor_id, updated_at DESC)`,
			`CREATE INDEX IF NOT EXISTS idx_terminal_persistent_sessions_target ON terminal_persistent_sessions(target)`,
		},
	})

	migrations = append(migrations, schemaMigration{
		Version: 48,
		Name:    "totp_columns",
		Statements: []string{
			`ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_secret TEXT`,
			`ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_verified_at TIMESTAMPTZ`,
			`ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_recovery_codes TEXT`,
		},
	})

	migrations = append(migrations, schemaMigration{
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
	})

	migrations = append(migrations, schemaMigration{
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
	})

	migrations = append(migrations, schemaMigration{
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
	})

	migrations = append(migrations, schemaMigration{
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
	})

	migrations = append(migrations, schemaMigration{
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
	})

	migrations = append(migrations, schemaMigration{
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
	})

	migrations = append(migrations, schemaMigration{
		Version: 55,
		Name:    "system_settings",
		Statements: []string{
			`CREATE TABLE IF NOT EXISTS system_settings (
				key        TEXT PRIMARY KEY,
				value      JSONB NOT NULL,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
			)`,
		},
	})

	migrations = append(migrations, schemaMigration{
		Version: 56,
		Name:    "add_manual_device_protocol_configs",
		Statements: []string{
			`ALTER TABLE assets ADD COLUMN IF NOT EXISTS host TEXT`,
			`CREATE TABLE IF NOT EXISTS asset_protocol_configs (
				id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
				asset_id TEXT NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
				protocol TEXT NOT NULL CHECK (protocol IN ('ssh', 'telnet', 'vnc', 'rdp', 'ard')),
				host TEXT NOT NULL DEFAULT '',
				port INTEGER NOT NULL DEFAULT 0,
				username TEXT NOT NULL DEFAULT '',
				credential_profile_id TEXT REFERENCES credential_profiles(id) ON DELETE SET NULL,
				enabled BOOLEAN NOT NULL DEFAULT true,
				last_tested_at TIMESTAMPTZ,
				test_status TEXT NOT NULL DEFAULT 'untested' CHECK (test_status IN ('untested', 'success', 'failed')),
				test_error TEXT,
				config JSONB NOT NULL DEFAULT '{}',
				created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
				updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
				UNIQUE(asset_id, protocol)
			)`,
			`CREATE INDEX IF NOT EXISTS idx_asset_protocol_configs_asset_id ON asset_protocol_configs(asset_id)`,
		},
	})

	migrations = append(migrations, schemaMigration{
		Version: 57,
		Name:    "groups_jump_chain",
		Statements: []string{
			`ALTER TABLE groups ADD COLUMN IF NOT EXISTS jump_chain JSONB`,
		},
	})

	migrations = append(migrations, schemaMigration{
		Version: 58,
		Name:    "workspace_tab_panel_sizes",
		Statements: []string{
			`ALTER TABLE terminal_workspace_tabs ADD COLUMN IF NOT EXISTS panel_sizes JSONB`,
		},
	})

	migrations = append(migrations, schemaMigration{
		Version: 59,
		Name:    "web_services_manual_nullable_host",
		Statements: []string{
			`ALTER TABLE web_services_manual ALTER COLUMN host_asset_id DROP NOT NULL`,
			`ALTER TABLE web_services_manual DROP CONSTRAINT IF EXISTS web_services_manual_host_asset_id_fkey`,
			`ALTER TABLE web_services_manual ADD CONSTRAINT web_services_manual_host_asset_id_fkey FOREIGN KEY (host_asset_id) REFERENCES assets(id) ON DELETE SET NULL`,
		},
	})

	migrations = append(migrations, schemaMigration{
		Version: 60,
		Name:    "rename_asset_dependencies_to_asset_edges_and_add_composites",
		Statements: []string{
			// Rename table
			`ALTER TABLE asset_dependencies RENAME TO asset_edges`,
			// Backward-compatibility view
			`CREATE VIEW asset_dependencies AS SELECT * FROM asset_edges`,
			// New columns on asset_edges
			`ALTER TABLE asset_edges ADD COLUMN origin TEXT NOT NULL DEFAULT 'manual'`,
			`ALTER TABLE asset_edges ADD COLUMN confidence DOUBLE PRECISION NOT NULL DEFAULT 1.0`,
			`ALTER TABLE asset_edges ADD COLUMN match_signals JSONB`,
			// Indexes for graph traversal
			`CREATE INDEX IF NOT EXISTS idx_asset_edges_source_type ON asset_edges(source_asset_id, relationship_type)`,
			`CREATE INDEX IF NOT EXISTS idx_asset_edges_target_type ON asset_edges(target_asset_id, relationship_type)`,
			`CREATE INDEX IF NOT EXISTS idx_asset_edges_origin ON asset_edges(origin) WHERE origin IN ('suggested', 'dismissed')`,
			// Composites table
			`CREATE TABLE IF NOT EXISTS asset_composites (
            composite_id    TEXT NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
            member_asset_id TEXT NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
            role            TEXT NOT NULL,
            created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
            PRIMARY KEY (composite_id, member_asset_id),
            CONSTRAINT uq_composite_member UNIQUE (member_asset_id)
        )`,
			`CREATE INDEX IF NOT EXISTS idx_composite_member ON asset_composites(member_asset_id)`,
			// Note: ADD COLUMN with DEFAULT already backfills all existing rows.
			// No explicit UPDATE needed — Postgres sets the default on all existing rows.
		},
	})

	migrations = append(migrations, schemaMigration{
		Version: 61,
		Name:    "migrate_link_suggestions_to_edge_proposals",
		Statements: []string{
			// Migrate pending suggestions as suggested edges
			// Note: confidence already uses 0.0-1.0 scale in both tables
			`INSERT INTO asset_edges (id, source_asset_id, target_asset_id, relationship_type, direction, criticality, origin, confidence, match_signals, metadata, created_at, updated_at)
         SELECT id, source_asset_id, target_asset_id, 'contains', 'downstream', 'medium', 'suggested', confidence, jsonb_build_object('match_reason', match_reason), '{}'::jsonb, created_at, created_at
         FROM asset_link_suggestions
         WHERE status = 'pending'
         ON CONFLICT DO NOTHING`,
			// Migrate dismissed suggestions as dismissed edges
			`INSERT INTO asset_edges (id, source_asset_id, target_asset_id, relationship_type, direction, criticality, origin, confidence, match_signals, metadata, created_at, updated_at)
         SELECT id, source_asset_id, target_asset_id, 'contains', 'downstream', 'medium', 'dismissed', confidence, jsonb_build_object('match_reason', match_reason), '{}'::jsonb, created_at, COALESCE(resolved_at, created_at)
         FROM asset_link_suggestions
         WHERE status = 'dismissed'
         ON CONFLICT DO NOTHING`,
			// For accepted suggestions: the edge already exists (created on accept).
			// Update those edges to set origin='manual' so they're properly tagged.
			`UPDATE asset_edges SET origin = 'manual'
         WHERE id IN (SELECT id FROM asset_link_suggestions WHERE status = 'accepted')
         AND origin = 'manual'`,
		},
	})

	migrations = append(migrations, schemaMigration{
		Version: 62,
		Name:    "terminal_session_bookmarks_and_scrollback",
		Statements: []string{
			`CREATE TABLE IF NOT EXISTS terminal_session_bookmarks (
            id TEXT PRIMARY KEY,
            actor_id TEXT NOT NULL,
            title TEXT NOT NULL,
            asset_id TEXT,
            host TEXT,
            port INT,
            username TEXT,
            credential_profile_id TEXT,
            jump_chain_group_id TEXT,
            tags JSONB NOT NULL DEFAULT '[]'::jsonb,
            created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
            updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
            last_used_at TIMESTAMPTZ
        )`,
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_terminal_bookmarks_actor_host ON terminal_session_bookmarks (actor_id, host, port, username)`,
			`CREATE INDEX IF NOT EXISTS idx_terminal_bookmarks_actor_recent ON terminal_session_bookmarks (actor_id, last_used_at DESC)`,

			`CREATE TABLE IF NOT EXISTS terminal_session_scrollback (
            persistent_session_id TEXT PRIMARY KEY,
            buffer BYTEA,
            buffer_size INT NOT NULL DEFAULT 0,
            total_lines INT NOT NULL DEFAULT 0,
            updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
        )`,

			`ALTER TABLE terminal_persistent_sessions ADD COLUMN IF NOT EXISTS bookmark_id TEXT`,
			`ALTER TABLE terminal_persistent_sessions ADD COLUMN IF NOT EXISTS archived_at TIMESTAMPTZ`,
			`ALTER TABLE terminal_persistent_sessions ADD COLUMN IF NOT EXISTS archive_after_days INT`,
			`ALTER TABLE terminal_persistent_sessions ADD COLUMN IF NOT EXISTS pinned BOOLEAN NOT NULL DEFAULT false`,
			`DROP INDEX IF EXISTS idx_terminal_persistent_sessions_actor_target`,
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_terminal_persistent_sessions_actor_target_tmux ON terminal_persistent_sessions (actor_id, target, tmux_session_name)`,
			`DO $$ BEGIN
            IF EXISTS (SELECT 1 FROM information_schema.check_constraints WHERE constraint_name LIKE '%terminal_persistent_sessions%status%') THEN
                ALTER TABLE terminal_persistent_sessions DROP CONSTRAINT IF EXISTS terminal_persistent_sessions_status_check;
            END IF;
        END $$`,
			`ALTER TABLE terminal_persistent_sessions ADD CONSTRAINT terminal_persistent_sessions_status_check CHECK (status IN ('attached', 'detached', 'archived'))`,
		},
	})

	migrations = append(migrations, schemaMigration{
		Version: 63,
		Name:    "file_connections_and_transfers",
		Statements: []string{
			`CREATE TABLE IF NOT EXISTS file_connections (
				id              TEXT PRIMARY KEY,
				name            TEXT NOT NULL,
				protocol        TEXT NOT NULL,
				host            TEXT NOT NULL,
				port            INT,
				initial_path    TEXT DEFAULT '/',
				credential_id   TEXT REFERENCES credential_profiles(id) ON DELETE SET NULL,
				extra_config    JSONB DEFAULT '{}',
				created_at      TIMESTAMPTZ DEFAULT NOW(),
				updated_at      TIMESTAMPTZ DEFAULT NOW()
			)`,
			`CREATE TABLE IF NOT EXISTS file_transfers (
				id                TEXT PRIMARY KEY,
				source_type       TEXT NOT NULL,
				source_id         TEXT NOT NULL,
				source_path       TEXT NOT NULL,
				dest_type         TEXT NOT NULL,
				dest_id           TEXT NOT NULL,
				dest_path         TEXT NOT NULL,
				file_name         TEXT NOT NULL,
				file_size         BIGINT,
				bytes_transferred BIGINT DEFAULT 0,
				status            TEXT DEFAULT 'pending',
				error             TEXT,
				started_at        TIMESTAMPTZ,
				completed_at      TIMESTAMPTZ
			)`,
			`CREATE INDEX IF NOT EXISTS idx_file_transfers_status ON file_transfers(status)`,
		},
	})

	migrations = append(migrations, schemaMigration{
		Version: 64,
		Name:    "metric_samples_labels",
		Statements: []string{
			`ALTER TABLE metric_samples ADD COLUMN IF NOT EXISTS labels JSONB`,
			`CREATE INDEX IF NOT EXISTS idx_metric_samples_labels ON metric_samples (asset_id, metric, collected_at DESC) WHERE labels IS NOT NULL`,
		},
	})

	// Migration 65: api_keys table.
	// NOTE: prefix is a 4-char display identifier, NOT a unique key.
	// Two keys can share the same prefix; use secret_hash for lookup.
	migrations = append(migrations, schemaMigration{
		Version: 65,
		Name:    "api_keys",
		Statements: []string{
			`CREATE TABLE IF NOT EXISTS api_keys (
				id TEXT PRIMARY KEY,
				name TEXT NOT NULL,
				prefix TEXT NOT NULL,
				secret_hash TEXT NOT NULL UNIQUE,
				role TEXT NOT NULL,
				scopes JSONB NOT NULL DEFAULT '[]',
				allowed_assets JSONB NOT NULL DEFAULT '[]',
				expires_at TIMESTAMPTZ,
				created_by TEXT NOT NULL,
				created_at TIMESTAMPTZ NOT NULL,
				last_used_at TIMESTAMPTZ
			)`,
			`CREATE INDEX IF NOT EXISTS idx_api_keys_secret_hash ON api_keys(secret_hash)`,
			`CREATE INDEX IF NOT EXISTS idx_api_keys_prefix ON api_keys(prefix)`,
		},
	})

	migrations = append(migrations, schemaMigration{
		Version: 66,
		Name:    "webhooks",
		Statements: []string{
			`CREATE TABLE IF NOT EXISTS webhooks (
				id TEXT PRIMARY KEY,
				name TEXT NOT NULL,
				url TEXT NOT NULL,
				secret TEXT,
				events JSONB NOT NULL DEFAULT '[]',
				enabled BOOLEAN NOT NULL DEFAULT true,
				created_by TEXT NOT NULL,
				created_at TIMESTAMPTZ NOT NULL,
				last_triggered_at TIMESTAMPTZ
			)`,
		},
	})

	migrations = append(migrations, schemaMigration{
		Version: 67,
		Name:    "scheduled_tasks",
		Statements: []string{
			`CREATE TABLE IF NOT EXISTS scheduled_tasks (
				id TEXT PRIMARY KEY,
				name TEXT NOT NULL,
				cron_expr TEXT NOT NULL,
				command TEXT NOT NULL,
				targets JSONB NOT NULL DEFAULT '[]',
				group_id TEXT,
				enabled BOOLEAN NOT NULL DEFAULT true,
				created_by TEXT NOT NULL,
				created_at TIMESTAMPTZ NOT NULL,
				last_run_at TIMESTAMPTZ,
				next_run_at TIMESTAMPTZ
			)`,
		},
	})

	migrations = append(migrations, schemaMigration{
		Version: 68,
		Name:    "saved_actions",
		Statements: []string{
			`CREATE TABLE IF NOT EXISTS saved_actions (
				id TEXT PRIMARY KEY,
				name TEXT NOT NULL,
				description TEXT,
				steps JSONB NOT NULL DEFAULT '[]',
				created_by TEXT NOT NULL,
				created_at TIMESTAMPTZ NOT NULL
			)`,
		},
	})

	// Migration 69: Migrate legacy config tables → asset_protocol_configs.
	// Copies asset_terminal_configs (SSH) and asset_desktop_configs (VNC) into the
	// unified protocol config table. Skips rows that already have a matching
	// (asset_id, protocol) entry to ensure idempotency.
	migrations = append(migrations, schemaMigration{
		Version: 69,
		Name:    "migrate_legacy_configs_to_protocol_configs",
		Statements: []string{
			// SSH configs from asset_terminal_configs.
			`INSERT INTO asset_protocol_configs (asset_id, protocol, host, port, username, credential_profile_id, enabled, config, created_at, updated_at)
			SELECT
				tc.asset_id,
				'ssh',
				tc.host,
				tc.port,
				COALESCE(tc.username, ''),
				tc.credential_profile_id,
				true,
				jsonb_build_object('strict_host_key', tc.strict_host_key, 'host_key', COALESCE(tc.host_key, '')),
				tc.updated_at,
				tc.updated_at
			FROM asset_terminal_configs tc
			WHERE NOT EXISTS (
				SELECT 1 FROM asset_protocol_configs pc
				WHERE pc.asset_id = tc.asset_id AND pc.protocol = 'ssh'
			)`,
			// VNC configs from asset_desktop_configs.
			`INSERT INTO asset_protocol_configs (asset_id, protocol, host, port, username, credential_profile_id, enabled, config, created_at, updated_at)
			SELECT
				dc.asset_id,
				'vnc',
				COALESCE((SELECT tc.host FROM asset_terminal_configs tc WHERE tc.asset_id = dc.asset_id), ''),
				dc.vnc_port,
				'',
				dc.credential_profile_id,
				true,
				'{}'::jsonb,
				dc.updated_at,
				dc.updated_at
			FROM asset_desktop_configs dc
			WHERE NOT EXISTS (
				SELECT 1 FROM asset_protocol_configs pc
				WHERE pc.asset_id = dc.asset_id AND pc.protocol = 'vnc'
			)`,
		},
	})

	migrations = append(migrations, schemaMigration{
		Version: 70,
		Name:    "add_synthetic_check_service_link",
		Statements: []string{
			`ALTER TABLE synthetic_checks ADD COLUMN IF NOT EXISTS service_id TEXT`,
			`CREATE INDEX IF NOT EXISTS idx_synthetic_checks_service_id ON synthetic_checks(service_id) WHERE service_id IS NOT NULL`,
		},
	})

	migrations = append(migrations, schemaMigration{
		Version: 71,
		Name:    "topology_canvas",
		Statements: []string{
			`CREATE TABLE IF NOT EXISTS topology_layouts (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            name TEXT NOT NULL DEFAULT 'My Homelab',
            viewport JSONB NOT NULL DEFAULT '{"x":0,"y":0,"zoom":1}',
            created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
            updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
        )`,
			`CREATE TABLE IF NOT EXISTS topology_zones (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            topology_id UUID NOT NULL REFERENCES topology_layouts(id) ON DELETE CASCADE,
            parent_zone_id UUID REFERENCES topology_zones(id) ON DELETE SET NULL,
            label TEXT NOT NULL,
            color TEXT NOT NULL DEFAULT 'blue',
            icon TEXT NOT NULL DEFAULT '',
            position JSONB NOT NULL DEFAULT '{"x":0,"y":0}',
            size JSONB NOT NULL DEFAULT '{"width":300,"height":200}',
            collapsed BOOLEAN NOT NULL DEFAULT false,
            sort_order INT NOT NULL DEFAULT 0,
            created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
            updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
        )`,
			`CREATE INDEX IF NOT EXISTS idx_topology_zones_topology ON topology_zones(topology_id)`,
			`CREATE INDEX IF NOT EXISTS idx_topology_zones_parent ON topology_zones(parent_zone_id)`,
			`CREATE TABLE IF NOT EXISTS zone_members (
            zone_id UUID NOT NULL REFERENCES topology_zones(id) ON DELETE CASCADE,
            asset_id TEXT NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
            position JSONB NOT NULL DEFAULT '{"x":0,"y":0}',
            sort_order INT NOT NULL DEFAULT 0,
            PRIMARY KEY (zone_id, asset_id)
        )`,
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_zone_members_asset ON zone_members(asset_id)`,
			`CREATE TABLE IF NOT EXISTS topology_connections (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            topology_id UUID NOT NULL REFERENCES topology_layouts(id) ON DELETE CASCADE,
            source_asset_id TEXT NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
            target_asset_id TEXT NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
            relationship TEXT NOT NULL,
            user_defined BOOLEAN NOT NULL DEFAULT true,
            label TEXT NOT NULL DEFAULT '',
            deleted BOOLEAN NOT NULL DEFAULT false,
            created_at TIMESTAMPTZ NOT NULL DEFAULT now()
        )`,
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_topology_connections_unique
            ON topology_connections(topology_id, source_asset_id, target_asset_id, relationship)
            WHERE deleted = false`,
			`CREATE INDEX IF NOT EXISTS idx_topology_connections_topology ON topology_connections(topology_id)`,
			`CREATE TABLE IF NOT EXISTS dismissed_assets (
            topology_id UUID NOT NULL REFERENCES topology_layouts(id) ON DELETE CASCADE,
            asset_id TEXT NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
            source TEXT NOT NULL DEFAULT '',
            type TEXT NOT NULL DEFAULT '',
            dismissed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
            PRIMARY KEY (topology_id, asset_id)
        )`,
		},
	})

	migrations = append(migrations, schemaMigration{
		Version: 72,
		Name:    "remote_bookmarks",
		Statements: []string{
			`CREATE TABLE IF NOT EXISTS remote_bookmarks (
				id TEXT PRIMARY KEY,
				label TEXT NOT NULL,
				protocol TEXT NOT NULL,
				host TEXT NOT NULL,
				port INTEGER NOT NULL,
				credential_id TEXT,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
			)`,
		},
	})

	return migrations
}

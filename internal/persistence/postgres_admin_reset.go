package persistence

import (
	"context"
	"time"
)

// ResetAllData truncates all operational/history data tables while preserving
// user accounts, sessions, settings, and schema migrations. Uses a single
// transaction with TRUNCATE ... CASCADE for atomicity.
func (s *PostgresStore) ResetAllData() (AdminResetResult, error) {
	ctx := context.Background()

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return AdminResetResult{}, err
	}
	defer tx.Rollback(ctx)

	// Truncate all data tables in one statement. CASCADE handles FK dependencies.
	// Preserved tables: users, sessions, schema_migrations, retention_settings, runtime_settings
	// Also preserved (user preferences): terminal_preferences, terminal_snippets, push_devices
	const stmt = `TRUNCATE
		notification_history, incident_alert_links, incident_events, incident_assets, incidents,
		alert_evaluations, alert_instances, alert_silences, alert_rule_targets, alert_routes, alert_rules,
		notification_channels,
		synthetic_check_results, synthetic_checks,
		hub_collectors,
		canonical_reconciliation_results, canonical_ingest_checkpoints, canonical_template_bindings,
		canonical_capability_sets, canonical_resource_relationships, resource_external_refs, provider_instances,
		asset_dependencies, asset_link_suggestions,
		terminal_commands, terminal_sessions, terminal_output_buffer, terminal_persistent_sessions,
		session_recordings,
		action_run_steps, action_runs,
		update_runs, update_plans,
		log_events, saved_log_views,
		metric_samples, asset_heartbeats, asset_terminal_configs, asset_desktop_configs,
		credential_profiles,
		audit_events,
		job_queue,
		enrollment_tokens, agent_tokens, agent_presence,
		web_services_manual, web_service_overrides, web_service_alt_urls,
		web_service_never_group_rules, web_service_url_grouping_settings,
		assets, groups
	CASCADE`

	if _, err := tx.Exec(ctx, stmt); err != nil {
		return AdminResetResult{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return AdminResetResult{}, err
	}

	return AdminResetResult{
		TablesCleared: 52,
		ResetAt:       time.Now().UTC(),
	}, nil
}

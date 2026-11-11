package persistence

// schemaMigrationAlertRuleKindAndScopeChecks lets the database store every
// rule kind and target scope the Hub accepts. The first alert_rules table
// allowed kinds without synthetic_check and scopes 'asset', 'site' and
// 'global', but the Hub has written 'group' since sites became groups, so a
// health-check rule or a rule for a group failed with an internal error.
// Rows still marked 'site' are rules for a group.
func schemaMigrationAlertRuleKindAndScopeChecks() schemaMigration {
	return schemaMigration{Version: 109, Name: "alert_rule_kind_and_scope_checks", Statements: []string{
		`ALTER TABLE alert_rules DROP CONSTRAINT IF EXISTS alert_rules_kind_check`,
		`ALTER TABLE alert_rules ADD CONSTRAINT alert_rules_kind_check
		 CHECK (kind IN ('metric_threshold', 'metric_deadman', 'heartbeat_stale', 'log_pattern', 'composite', 'synthetic_check'))`,
		`ALTER TABLE alert_rules DROP CONSTRAINT IF EXISTS alert_rules_target_scope_check`,
		`UPDATE alert_rules SET target_scope = 'group' WHERE target_scope = 'site'`,
		`ALTER TABLE alert_rules ADD CONSTRAINT alert_rules_target_scope_check
		 CHECK (target_scope IN ('asset', 'group', 'global'))`,
	}}
}

package persistence

// Schema changes owned by the incidents domain.

func schemaMigrationIncidentsAndAlertLinks() schemaMigration {
	return schemaMigration{
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
	}
}

func schemaMigrationIncidentEvents() schemaMigration {
	return schemaMigration{
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
	}
}

func schemaMigrationIncidentPostmortem() schemaMigration {
	return schemaMigration{
		Version: 18,
		Name:    "incident_postmortem",
		Statements: []string{
			`ALTER TABLE incidents ADD COLUMN IF NOT EXISTS root_cause TEXT DEFAULT ''`,
			`ALTER TABLE incidents ADD COLUMN IF NOT EXISTS action_items JSONB DEFAULT '[]'`,
			`ALTER TABLE incidents ADD COLUMN IF NOT EXISTS lessons_learned TEXT DEFAULT ''`,
		},
	}
}

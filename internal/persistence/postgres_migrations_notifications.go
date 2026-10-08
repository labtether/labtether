package persistence

// Schema changes owned by the notifications domain.

func schemaMigrationNotificationChannelsRoutesHistory() schemaMigration {
	return schemaMigration{
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
	}
}

func schemaMigrationNotificationHistoryRetryColumns() schemaMigration {
	return schemaMigration{
		Version: 33,
		Name:    "notification_history_retry_columns",
		Statements: []string{
			`ALTER TABLE notification_history ADD COLUMN IF NOT EXISTS retry_count integer NOT NULL DEFAULT 0`,
			`ALTER TABLE notification_history ADD COLUMN IF NOT EXISTS max_retries integer NOT NULL DEFAULT 3`,
			`ALTER TABLE notification_history ADD COLUMN IF NOT EXISTS next_retry_at timestamptz`,
			`CREATE INDEX IF NOT EXISTS idx_notification_history_pending_retry ON notification_history(next_retry_at) WHERE status = 'failed'`,
		},
	}
}

func schemaMigrationPushDevices() schemaMigration {
	return schemaMigration{
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
	}
}

func schemaMigrationNotificationHistoryPayload() schemaMigration {
	return schemaMigration{
		Version: 74,
		Name:    "notification_history_payload",
		Statements: []string{
			`ALTER TABLE notification_history ADD COLUMN IF NOT EXISTS payload JSONB NOT NULL DEFAULT '{}'::jsonb`,
			`UPDATE notification_history SET payload = '{}'::jsonb WHERE payload IS NULL`,
		},
	}
}

func schemaMigrationPushDeviceNotificationPreferences() schemaMigration {
	return schemaMigration{
		Version: 82,
		Name:    "push_device_notification_preferences",
		Statements: []string{
			`ALTER TABLE push_devices ADD COLUMN IF NOT EXISTS environment TEXT NOT NULL DEFAULT '' CHECK (environment IN ('', 'sandbox', 'production'))`,
			`ALTER TABLE push_devices ADD COLUMN IF NOT EXISTS notify_critical_alerts BOOLEAN NOT NULL DEFAULT TRUE`,
			`ALTER TABLE push_devices ADD COLUMN IF NOT EXISTS notify_node_offline BOOLEAN NOT NULL DEFAULT TRUE`,
			`ALTER TABLE push_devices ADD COLUMN IF NOT EXISTS notify_service_down BOOLEAN NOT NULL DEFAULT TRUE`,
			`ALTER TABLE push_devices ADD COLUMN IF NOT EXISTS push_category TEXT NOT NULL DEFAULT 'critical_only' CHECK (push_category IN ('critical_only', 'all_alerts', 'alerts_and_incidents'))`,
			`ALTER TABLE push_devices ADD COLUMN IF NOT EXISTS minimum_severity TEXT NOT NULL DEFAULT 'warning' CHECK (minimum_severity IN ('info', 'warning', 'high', 'critical'))`,
			`ALTER TABLE push_devices ADD COLUMN IF NOT EXISTS quiet_hours_enabled BOOLEAN NOT NULL DEFAULT FALSE`,
			`ALTER TABLE push_devices ADD COLUMN IF NOT EXISTS quiet_hours_start_minutes INTEGER NOT NULL DEFAULT 1320 CHECK (quiet_hours_start_minutes BETWEEN 0 AND 1439)`,
			`ALTER TABLE push_devices ADD COLUMN IF NOT EXISTS quiet_hours_end_minutes INTEGER NOT NULL DEFAULT 420 CHECK (quiet_hours_end_minutes BETWEEN 0 AND 1439)`,
			`ALTER TABLE push_devices ADD COLUMN IF NOT EXISTS digest_window_seconds INTEGER NOT NULL DEFAULT 180 CHECK (digest_window_seconds BETWEEN 30 AND 86400)`,
		},
	}
}

func schemaMigrationPushDeviceTimezoneAndTokenOwnership() schemaMigration {
	return schemaMigration{
		Version: 84,
		Name:    "push_device_timezone_and_token_ownership",
		Statements: []string{
			`ALTER TABLE push_devices ADD COLUMN IF NOT EXISTS time_zone TEXT NOT NULL DEFAULT 'UTC'`,
			`DELETE FROM push_devices stale
			 USING push_devices winner
			 WHERE stale.id <> winner.id
			   AND stale.device_id = winner.device_id
			   AND stale.bundle_id = winner.bundle_id
			   AND stale.environment = winner.environment
			   AND (stale.updated_at < winner.updated_at OR (stale.updated_at = winner.updated_at AND stale.id < winner.id))`,
			`DELETE FROM push_devices stale
			 USING push_devices winner
			 WHERE stale.id <> winner.id
			   AND stale.push_token = winner.push_token
			   AND stale.bundle_id = winner.bundle_id
			   AND stale.environment = winner.environment
			   AND (stale.updated_at < winner.updated_at OR (stale.updated_at = winner.updated_at AND stale.id < winner.id))`,
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_push_devices_device_topic_environment
				ON push_devices(device_id, bundle_id, environment)`,
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_push_devices_token_topic_environment
				ON push_devices(push_token, bundle_id, environment)`,
		},
	}
}

func schemaMigrationPushDeviceTimezoneUnknownDefault() schemaMigration {
	return schemaMigration{
		Version: 86,
		Name:    "push_device_timezone_unknown_default",
		Statements: []string{
			`ALTER TABLE push_devices ALTER COLUMN time_zone SET DEFAULT ''`,
		},
	}
}

func schemaMigrationLiveActivityPushTokens() schemaMigration {
	return schemaMigration{
		Version: 89,
		Name:    "live_activity_push_tokens",
		Statements: []string{
			`CREATE TABLE IF NOT EXISTS live_activity_push_tokens (
				id TEXT PRIMARY KEY,
				user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				device_id TEXT NOT NULL,
				activity_id TEXT NOT NULL,
				incident_id TEXT NOT NULL,
				token_ciphertext TEXT NOT NULL,
				token_hash TEXT NOT NULL,
				bundle_id TEXT NOT NULL,
				environment TEXT NOT NULL CHECK (environment IN ('sandbox', 'production')),
				show_full_details BOOLEAN NOT NULL DEFAULT FALSE,
					retry_count INTEGER NOT NULL DEFAULT 0 CHECK (retry_count BETWEEN 0 AND 255),
					delivery_generation BIGINT NOT NULL DEFAULT 0,
				next_retry_at TIMESTAMPTZ,
					pending_state_ciphertext TEXT NOT NULL DEFAULT '',
					last_delivered_incident_updated_at TIMESTAMPTZ,
				expires_at TIMESTAMPTZ NOT NULL,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				UNIQUE(user_id, device_id, activity_id),
				UNIQUE(token_hash, bundle_id, environment)
			)`,
			`CREATE INDEX IF NOT EXISTS idx_live_activity_tokens_incident_expiry
				ON live_activity_push_tokens(incident_id, expires_at)`,
			`CREATE INDEX IF NOT EXISTS idx_live_activity_tokens_retry
				ON live_activity_push_tokens(next_retry_at)
				WHERE next_retry_at IS NOT NULL`,
			`CREATE INDEX IF NOT EXISTS idx_live_activity_tokens_expiry
				ON live_activity_push_tokens(expires_at)`,
		},
	}
}

func schemaMigrationDurablePushAlertDigests() schemaMigration {
	return schemaMigration{
		Version: 91,
		Name:    "durable_push_alert_digests",
		Statements: []string{
			`CREATE TABLE IF NOT EXISTS push_alert_digest_states (
				push_device_id TEXT PRIMARY KEY REFERENCES push_devices(id) ON DELETE CASCADE,
				channel_id TEXT NOT NULL REFERENCES notification_channels(id) ON DELETE CASCADE,
				window_seconds INTEGER NOT NULL CHECK (window_seconds BETWEEN 30 AND 86400),
				due_at TIMESTAMPTZ NOT NULL,
				expires_at TIMESTAMPTZ NOT NULL,
				retry_count INTEGER NOT NULL DEFAULT 0 CHECK (retry_count BETWEEN 0 AND 8),
				delivery_generation BIGINT NOT NULL DEFAULT 0,
				lease_expires_at TIMESTAMPTZ,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				CHECK (expires_at > created_at)
			)`,
			`CREATE INDEX IF NOT EXISTS idx_push_alert_digest_states_due
				ON push_alert_digest_states(due_at ASC, push_device_id ASC)`,
			`CREATE INDEX IF NOT EXISTS idx_push_alert_digest_states_expiry
				ON push_alert_digest_states(expires_at ASC)`,
			`CREATE TABLE IF NOT EXISTS push_alert_digest_events (
				id TEXT PRIMARY KEY,
				push_device_id TEXT NOT NULL REFERENCES push_alert_digest_states(push_device_id) ON DELETE CASCADE,
				dedupe_key TEXT NOT NULL CHECK (dedupe_key ~ '^[0-9a-f]{64}$'),
				severity TEXT NOT NULL CHECK (severity IN ('info', 'warning')),
				node_offline BOOLEAN NOT NULL DEFAULT FALSE,
				service_down BOOLEAN NOT NULL DEFAULT FALSE,
				group_ids JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(group_ids) = 'array'),
				maintenance_scope_complete BOOLEAN NOT NULL DEFAULT TRUE,
				expires_at TIMESTAMPTZ NOT NULL,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				UNIQUE(push_device_id, dedupe_key),
				CHECK (expires_at > created_at)
			)`,
			`CREATE INDEX IF NOT EXISTS idx_push_alert_digest_events_device_time
				ON push_alert_digest_events(push_device_id, created_at ASC, id ASC)`,
			`CREATE INDEX IF NOT EXISTS idx_push_alert_digest_events_expiry
				ON push_alert_digest_events(expires_at ASC)`,
		},
	}
}

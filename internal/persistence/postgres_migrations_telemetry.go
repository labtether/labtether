package persistence

// Schema changes owned by the telemetry domain.

func schemaMigrationMetricSamplesLabels() schemaMigration {
	return schemaMigration{
		Version: 64,
		Name:    "metric_samples_labels",
		Statements: []string{
			`ALTER TABLE metric_samples ADD COLUMN IF NOT EXISTS labels JSONB`,
			`CREATE INDEX IF NOT EXISTS idx_metric_samples_labels ON metric_samples (asset_id, metric, collected_at DESC) WHERE labels IS NOT NULL`,
		},
	}
}

func schemaMigrationHubMetricSamples() schemaMigration {
	return schemaMigration{
		Version: 87,
		Name:    "hub_metric_samples",
		Statements: []string{
			`CREATE TABLE IF NOT EXISTS hub_metric_samples (
				id BIGSERIAL PRIMARY KEY,
				scope TEXT NOT NULL CHECK (scope IN ('hub-alerts', 'hub-reliability')),
				metric TEXT NOT NULL,
				unit TEXT NOT NULL,
				value DOUBLE PRECISION NOT NULL,
				collected_at TIMESTAMPTZ NOT NULL,
				labels JSONB
			)`,
			`CREATE INDEX IF NOT EXISTS idx_hub_metric_samples_scope_metric_time
				ON hub_metric_samples(scope, metric, collected_at DESC)`,
			`CREATE INDEX IF NOT EXISTS idx_hub_metric_samples_time
				ON hub_metric_samples(collected_at DESC)`,
		},
	}
}

func schemaMigrationMetricSamplesBoundedLabeledSnapshotIndex() schemaMigration {
	return schemaMigration{
		Version: 88,
		Name:    "metric_samples_bounded_labeled_snapshot_index",
		Statements: []string{
			`CREATE INDEX IF NOT EXISTS idx_metric_samples_asset_time_labeled_snapshot
				ON metric_samples(asset_id, collected_at DESC, id DESC)`,
		},
	}
}

func schemaMigrationPrometheusRemoteWriteReplayState() schemaMigration {
	return schemaMigration{
		Version: 94,
		Name:    "prometheus_remote_write_replay_state",
		Statements: []string{
			`CREATE TABLE IF NOT EXISTS prometheus_remote_write_state (
				singleton BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (singleton),
				endpoint_fingerprint TEXT NOT NULL CHECK (endpoint_fingerprint ~ '^[0-9a-f]{64}$'),
				asset_sample_id BIGINT NOT NULL DEFAULT 0 CHECK (asset_sample_id >= 0),
				hub_sample_id BIGINT NOT NULL DEFAULT 0 CHECK (hub_sample_id >= 0),
				last_advanced_at TIMESTAMPTZ,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
			)`,
		},
	}
}

func schemaMigrationHubSyntheticMetricScope() schemaMigration {
	return schemaMigration{
		Version: 96,
		Name:    "hub_synthetic_metric_scope",
		Statements: []string{
			`ALTER TABLE hub_metric_samples DROP CONSTRAINT IF EXISTS hub_metric_samples_scope_check`,
			`ALTER TABLE hub_metric_samples ADD CONSTRAINT hub_metric_samples_scope_check
				CHECK (scope IN ('hub-alerts', 'hub-reliability', 'hub-synthetic'))`,
		},
	}
}

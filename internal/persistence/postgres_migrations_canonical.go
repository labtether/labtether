package persistence

// Schema changes owned by the canonical domain.

func schemaMigrationCanonicalModelPersistence() schemaMigration {
	return schemaMigration{
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
	}
}

func schemaMigrationAddHostedOnRelationshipType() schemaMigration {
	return schemaMigration{
		Version: 32,
		Name:    "add_hosted_on_relationship_type",
		Statements: []string{
			`ALTER TABLE asset_dependencies DROP CONSTRAINT asset_dependencies_relationship_type_check`,
			`ALTER TABLE asset_dependencies ADD CONSTRAINT asset_dependencies_relationship_type_check CHECK (relationship_type IN ('runs_on', 'hosted_on', 'depends_on', 'provides_to', 'connected_to'))`,
		},
	}
}

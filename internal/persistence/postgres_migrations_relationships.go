package persistence

// Schema changes owned by the relationships domain.

func schemaMigrationAssetDependenciesIncidentAssets() schemaMigration {
	return schemaMigration{
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
	}
}

func schemaMigrationRenameAssetDependenciesToAssetEdgesAndAddComposites() schemaMigration {
	return schemaMigration{
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
	}
}

func schemaMigrationMigrateLinkSuggestionsToEdgeProposals() schemaMigration {
	return schemaMigration{
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
	}
}

func schemaMigrationTopologyCanvas() schemaMigration {
	return schemaMigration{
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
	}
}

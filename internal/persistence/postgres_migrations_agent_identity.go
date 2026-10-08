package persistence

// Schema changes owned by the agent identity domain.

func schemaMigrationEnrollmentAndAgentTokens() schemaMigration {
	return schemaMigration{
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
	}
}

func schemaMigrationAgentPresence() schemaMigration {
	return schemaMigration{
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
	}
}

func schemaMigrationAgentTokensExpiry() schemaMigration {
	return schemaMigration{
		Version: 29,
		Name:    "agent_tokens_expiry",
		Statements: []string{
			`ALTER TABLE agent_tokens ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ`,
			`UPDATE agent_tokens SET expires_at = created_at + INTERVAL '30 days' WHERE expires_at IS NULL`,
			`ALTER TABLE agent_tokens ALTER COLUMN expires_at SET NOT NULL`,
			`CREATE INDEX IF NOT EXISTS idx_agent_tokens_expires ON agent_tokens(expires_at)`,
		},
	}
}

func schemaMigrationNormalizeAgentSource() schemaMigration {
	return schemaMigration{
		Version: 31,
		Name:    "normalize_agent_source",
		Statements: []string{
			`UPDATE assets SET source = 'agent' WHERE source = 'labtether-agent'`,
			`UPDATE provider_instances SET provider = 'agent' WHERE provider = 'labtether-agent'`,
		},
	}
}

func schemaMigrationPreparedAgentApprovalTokens() schemaMigration {
	return schemaMigration{
		Version: 92,
		Name:    "prepared_agent_approval_tokens",
		Statements: []string{
			`ALTER TABLE agent_tokens DROP CONSTRAINT IF EXISTS agent_tokens_status_check`,
			`ALTER TABLE agent_tokens ADD CONSTRAINT agent_tokens_status_check CHECK (status IN ('pending', 'active', 'revoked'))`,
		},
	}
}

func schemaMigrationDurableAgentIdentityState() schemaMigration {
	return schemaMigration{
		Version: 93,
		Name:    "durable_agent_identity_state",
		Statements: []string{
			`CREATE TABLE IF NOT EXISTS agent_identity_state (
				asset_id TEXT PRIMARY KEY REFERENCES assets(id) ON DELETE CASCADE,
				credential_rotated_at TIMESTAMPTZ NOT NULL
			)`,
			`INSERT INTO agent_identity_state (asset_id, credential_rotated_at)
			 SELECT a.id, GREATEST(a.created_at, COALESCE(MAX(t.created_at), a.created_at))
			 FROM assets a
			 LEFT JOIN agent_tokens t ON t.asset_id = a.id
			 WHERE LOWER(BTRIM(a.source)) = 'agent' OR t.asset_id IS NOT NULL
			 GROUP BY a.id, a.created_at
			 ON CONFLICT (asset_id) DO UPDATE SET
				credential_rotated_at = GREATEST(agent_identity_state.credential_rotated_at, EXCLUDED.credential_rotated_at)`,
		},
	}
}

func schemaMigrationRetiredAgentIdentities() schemaMigration {
	return schemaMigration{
		Version: 97,
		Name:    "retired_agent_identities",
		Statements: []string{
			`CREATE TABLE IF NOT EXISTS retired_agent_identities (
				asset_id TEXT PRIMARY KEY,
				retired_at TIMESTAMPTZ NOT NULL,
				CHECK (BTRIM(asset_id) <> '')
			)`,
		},
	}
}

// Version 97 belongs to retired-agent identity tombstones. Keep this
// independently reviewable enrollment change at version 98.
func schemaMigrationScopedEnrollmentTokens() schemaMigration {
	return schemaMigration{
		Version: 98,
		Name:    "scoped_enrollment_tokens",
		Statements: []string{
			`ALTER TABLE enrollment_tokens ADD COLUMN IF NOT EXISTS scope TEXT`,
			`ALTER TABLE enrollment_tokens ADD COLUMN IF NOT EXISTS asset_id TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE enrollment_tokens ADD COLUMN IF NOT EXISTS allowed_group_id TEXT`,
			`ALTER TABLE enrollment_tokens ADD COLUMN IF NOT EXISTS created_by TEXT NOT NULL DEFAULT ''`,
			`UPDATE enrollment_tokens
			 SET scope = 'legacy_revoked', revoked_at = COALESCE(revoked_at, clock_timestamp())
			 WHERE scope IS NULL`,
			`ALTER TABLE enrollment_tokens ALTER COLUMN scope SET NOT NULL`,
			`ALTER TABLE enrollment_tokens DROP CONSTRAINT IF EXISTS enrollment_tokens_scope_check`,
			`ALTER TABLE enrollment_tokens ADD CONSTRAINT enrollment_tokens_scope_check CHECK (
				(scope = 'asset' AND asset_id <> '' AND max_uses = 1) OR
				(scope = 'group' AND asset_id = '' AND allowed_group_id IS NOT NULL AND allowed_group_id <> '') OR
				(scope IN ('unplaced', 'unrestricted') AND asset_id = '' AND allowed_group_id IS NULL) OR
				(scope = 'legacy_revoked' AND revoked_at IS NOT NULL)
			)`,
			`ALTER TABLE enrollment_tokens DROP CONSTRAINT IF EXISTS enrollment_tokens_creator_check`,
			`ALTER TABLE enrollment_tokens ADD CONSTRAINT enrollment_tokens_creator_check CHECK (
				scope = 'legacy_revoked' OR created_by <> ''
			)`,
			`CREATE INDEX IF NOT EXISTS idx_enrollment_tokens_allowed_group ON enrollment_tokens(allowed_group_id) WHERE allowed_group_id IS NOT NULL`,
		},
	}
}

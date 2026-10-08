package persistence

// Schema changes owned by the remote access domain.

func schemaMigrationAddManualDeviceProtocolConfigs() schemaMigration {
	return schemaMigration{
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
	}
}

func schemaMigrationFileConnectionsAndTransfers() schemaMigration {
	return schemaMigration{
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
				actor_id          TEXT NOT NULL DEFAULT '',
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
	}
}

// Copy legacy SSH/VNC settings into protocol configs. Existing matching
// (asset_id, protocol) rows are skipped to preserve idempotency.
func schemaMigrationMigrateLegacyConfigsToProtocolConfigs() schemaMigration {
	return schemaMigration{
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
	}
}

func schemaMigrationRemoteBookmarks() schemaMigration {
	return schemaMigration{
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
	}
}

func schemaMigrationFileTransfersActorScope() schemaMigration {
	return schemaMigration{
		Version: 77,
		Name:    "file_transfers_actor_scope",
		Statements: []string{
			`ALTER TABLE file_transfers ADD COLUMN IF NOT EXISTS actor_id TEXT NOT NULL DEFAULT ''`,
			`UPDATE file_transfers SET actor_id = '' WHERE actor_id IS NULL`,
			`CREATE INDEX IF NOT EXISTS idx_file_transfers_actor_id ON file_transfers(actor_id)`,
		},
	}
}

func schemaMigrationFileConnectionsActorScope() schemaMigration {
	return schemaMigration{
		Version: 80,
		Name:    "file_connections_actor_scope",
		Statements: []string{
			`ALTER TABLE file_connections ADD COLUMN IF NOT EXISTS actor_id TEXT NOT NULL DEFAULT ''`,
			`CREATE INDEX IF NOT EXISTS idx_file_connections_actor_updated ON file_connections(actor_id, updated_at DESC)`,
		},
	}
}

func schemaMigrationFileConnectionsActorBackfillSingleOwner() schemaMigration {
	return schemaMigration{
		Version: 81,
		Name:    "file_connections_actor_backfill_single_owner",
		Statements: []string{
			`DO $$
			DECLARE
				owner_count INTEGER := 0;
				legacy_owner_id TEXT := '';
			BEGIN
				IF EXISTS (
					SELECT 1
					  FROM information_schema.columns
					 WHERE table_name = 'users'
					   AND column_name = 'role'
				) THEN
					SELECT COUNT(*), COALESCE(MIN(id), '')
					  INTO owner_count, legacy_owner_id
					  FROM users
					 WHERE role = 'owner';

					IF owner_count = 1 AND legacy_owner_id <> '' THEN
						UPDATE file_connections
						   SET actor_id = legacy_owner_id
						 WHERE actor_id = '';
					END IF;
				END IF;
			END $$`,
		},
	}
}

func schemaMigrationCredentialProfileLifecycleGuards() schemaMigration {
	return schemaMigration{
		Version: 95,
		Name:    "credential_profile_lifecycle_guards",
		Statements: []string{
			`ALTER TABLE credential_profiles ADD COLUMN IF NOT EXISTS created_by TEXT NOT NULL DEFAULT ''`,
			`CREATE INDEX IF NOT EXISTS idx_credential_profiles_created_by ON credential_profiles(created_by, created_at DESC)`,
			`CREATE INDEX IF NOT EXISTS idx_asset_desktop_configs_credential ON asset_desktop_configs(credential_profile_id)`,
			`CREATE INDEX IF NOT EXISTS idx_asset_protocol_configs_credential ON asset_protocol_configs(credential_profile_id)`,
			`CREATE INDEX IF NOT EXISTS idx_terminal_session_bookmarks_credential ON terminal_session_bookmarks(credential_profile_id)`,
			`CREATE INDEX IF NOT EXISTS idx_remote_bookmarks_credential ON remote_bookmarks(credential_id)`,
			`CREATE INDEX IF NOT EXISTS idx_file_connections_credential ON file_connections(credential_id)`,
			`CREATE INDEX IF NOT EXISTS idx_hub_collectors_credential ON hub_collectors((config->>'credential_id'))`,
			`CREATE INDEX IF NOT EXISTS idx_groups_jump_chain_gin ON groups USING GIN (jump_chain jsonb_path_ops) WHERE jump_chain IS NOT NULL`,
		},
	}
}

func schemaMigrationRemoteBookmarkSpiceTransportSecurity() schemaMigration {
	return schemaMigration{
		Version: 100,
		Name:    "remote_bookmark_spice_transport_security",
		Statements: []string{
			`ALTER TABLE remote_bookmarks ADD COLUMN IF NOT EXISTS spice_security_mode TEXT NOT NULL DEFAULT 'tls'`,
			`ALTER TABLE remote_bookmarks ADD COLUMN IF NOT EXISTS spice_ca_pem TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE remote_bookmarks DROP CONSTRAINT IF EXISTS remote_bookmarks_spice_security_mode_check`,
			`ALTER TABLE remote_bookmarks ADD CONSTRAINT remote_bookmarks_spice_security_mode_check
				CHECK (spice_security_mode IN ('tls', 'cleartext'))`,
		},
	}
}

func schemaMigrationRemoteBookmarkVncTransportSecurity() schemaMigration {
	return schemaMigration{
		Version: 101,
		Name:    "remote_bookmark_vnc_transport_security",
		Statements: []string{
			`ALTER TABLE remote_bookmarks ADD COLUMN IF NOT EXISTS allow_insecure_vnc BOOLEAN NOT NULL DEFAULT false`,
		},
	}
}

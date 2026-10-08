package persistence

// Schema changes owned by the terminal domain.

func schemaMigrationTerminalCredentialsAndAssetTerminalConfig() schemaMigration {
	return schemaMigration{
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
	}
}

func schemaMigrationAssetDesktopConfigs() schemaMigration {
	return schemaMigration{
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
	}
}

func schemaMigrationTerminalUplift() schemaMigration {
	return schemaMigration{
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
	}
}

func schemaMigrationSessionRecordings() schemaMigration {
	return schemaMigration{
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
	}
}

func schemaMigrationTerminalPersistentSessions() schemaMigration {
	return schemaMigration{
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
	}
}

func schemaMigrationTerminalSessionBookmarksAndScrollback() schemaMigration {
	return schemaMigration{
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
	}
}

func schemaMigrationTerminalUserStateActorScope() schemaMigration {
	return schemaMigration{
		Version: 83,
		Name:    "terminal_user_state_actor_scope",
		Statements: []string{
			`ALTER TABLE terminal_workspace_tabs ADD COLUMN IF NOT EXISTS actor_id TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE terminal_snippets ADD COLUMN IF NOT EXISTS actor_id TEXT NOT NULL DEFAULT ''`,
			`DO $$
			DECLARE
				owner_count INTEGER := 0;
				legacy_actor_id TEXT := 'owner';
			BEGIN
				IF EXISTS (
					SELECT 1
					  FROM information_schema.columns
					 WHERE table_name = 'users'
					   AND column_name = 'role'
				) THEN
					SELECT COUNT(*), COALESCE(MIN(id), 'owner')
					  INTO owner_count, legacy_actor_id
					  FROM users
					 WHERE role = 'owner';

					IF owner_count <> 1 OR legacy_actor_id = '' THEN
						legacy_actor_id := 'owner';
					END IF;
				END IF;

				UPDATE terminal_workspace_tabs
				   SET actor_id = legacy_actor_id
				 WHERE actor_id = '';
				UPDATE terminal_snippets
				   SET actor_id = legacy_actor_id
				 WHERE actor_id = '';

				IF legacy_actor_id <> 'default' THEN
					INSERT INTO terminal_preferences (
						user_id, theme, font_family, font_size, cursor_style,
						cursor_blink, scrollback, toolbar_keys, auto_reconnect, updated_at
					)
					SELECT legacy_actor_id, theme, font_family, font_size, cursor_style,
						cursor_blink, scrollback, toolbar_keys, auto_reconnect, updated_at
					  FROM terminal_preferences
					 WHERE user_id = 'default'
					ON CONFLICT (user_id) DO NOTHING;

					DELETE FROM terminal_preferences WHERE user_id = 'default';
				END IF;
			END $$`,
			`ALTER TABLE terminal_workspace_tabs ALTER COLUMN actor_id DROP DEFAULT`,
			`ALTER TABLE terminal_snippets ALTER COLUMN actor_id DROP DEFAULT`,
			`CREATE INDEX IF NOT EXISTS idx_terminal_workspace_tabs_actor_order ON terminal_workspace_tabs(actor_id, sort_order, created_at)`,
			`CREATE INDEX IF NOT EXISTS idx_terminal_snippets_actor_scope_order ON terminal_snippets(actor_id, scope, sort_order, created_at)`,
		},
	}
}

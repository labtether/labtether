package persistence

// Schema changes owned by the authentication domain.

func schemaMigrationAuthUsersSessions() schemaMigration {
	return schemaMigration{
		Version: 10,
		Name:    "auth_users_sessions",
		Statements: []string{
			`CREATE TABLE IF NOT EXISTS users (
					id TEXT PRIMARY KEY,
					username TEXT NOT NULL UNIQUE,
					password_hash TEXT NOT NULL,
					created_at TIMESTAMPTZ NOT NULL,
					updated_at TIMESTAMPTZ NOT NULL
				)`,
			`CREATE TABLE IF NOT EXISTS sessions (
					id TEXT PRIMARY KEY,
					user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
					token_hash TEXT NOT NULL,
					expires_at TIMESTAMPTZ NOT NULL,
					created_at TIMESTAMPTZ NOT NULL
				)`,
			`CREATE INDEX IF NOT EXISTS idx_sessions_token_hash ON sessions(token_hash)`,
			`CREATE INDEX IF NOT EXISTS idx_sessions_user_expires ON sessions(user_id, expires_at DESC)`,
			`CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at)`,
		},
	}
}

func schemaMigrationAuthRolesAndOidcIdentity() schemaMigration {
	return schemaMigration{
		Version: 42,
		Name:    "auth_roles_and_oidc_identity",
		Statements: []string{
			`ALTER TABLE users ADD COLUMN IF NOT EXISTS role TEXT NOT NULL DEFAULT 'owner'`,
			`ALTER TABLE users ADD COLUMN IF NOT EXISTS auth_provider TEXT NOT NULL DEFAULT 'local'`,
			`ALTER TABLE users ADD COLUMN IF NOT EXISTS oidc_subject TEXT`,
			`UPDATE users
			 SET role = 'owner'
			 WHERE role IS NULL OR BTRIM(role) = ''`,
			`UPDATE users
			 SET auth_provider = 'local'
			 WHERE auth_provider IS NULL OR BTRIM(auth_provider) = ''`,
			`CREATE INDEX IF NOT EXISTS idx_users_role ON users(role)`,
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_users_auth_provider_subject
				ON users(auth_provider, oidc_subject)
				WHERE oidc_subject IS NOT NULL`,
		},
	}
}

func schemaMigrationTotpColumns() schemaMigration {
	return schemaMigration{
		Version: 48,
		Name:    "totp_columns",
		Statements: []string{
			`ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_secret TEXT`,
			`ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_verified_at TIMESTAMPTZ`,
			`ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_recovery_codes TEXT`,
		},
	}
}

// The prefix is a four-character display identifier, not a unique key.
// Two keys can share a prefix; use secret_hash for lookup.
func schemaMigrationApiKeys() schemaMigration {
	return schemaMigration{
		Version: 65,
		Name:    "api_keys",
		Statements: []string{
			`CREATE TABLE IF NOT EXISTS api_keys (
				id TEXT PRIMARY KEY,
				name TEXT NOT NULL,
				prefix TEXT NOT NULL,
				secret_hash TEXT NOT NULL UNIQUE,
				role TEXT NOT NULL,
				scopes JSONB NOT NULL DEFAULT '[]',
				allowed_assets JSONB NOT NULL DEFAULT '[]',
				expires_at TIMESTAMPTZ,
				created_by TEXT NOT NULL,
				created_at TIMESTAMPTZ NOT NULL,
				last_used_at TIMESTAMPTZ
			)`,
			`CREATE INDEX IF NOT EXISTS idx_api_keys_secret_hash ON api_keys(secret_hash)`,
			`CREATE INDEX IF NOT EXISTS idx_api_keys_prefix ON api_keys(prefix)`,
		},
	}
}

func schemaMigrationOidcIssuerScopedIdentity() schemaMigration {
	return schemaMigration{
		Version: 85,
		Name:    "oidc_issuer_scoped_identity",
		Statements: []string{
			`ALTER TABLE users ADD COLUMN IF NOT EXISTS oidc_issuer TEXT`,
			`DROP INDEX IF EXISTS idx_users_auth_provider_subject`,
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_users_auth_provider_issuer_subject
				ON users(auth_provider, oidc_issuer, oidc_subject)
				WHERE oidc_subject IS NOT NULL
				  AND oidc_issuer IS NOT NULL
				  AND BTRIM(oidc_issuer) <> ''`,
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_users_auth_provider_legacy_subject
				ON users(auth_provider, oidc_subject)
				WHERE oidc_subject IS NOT NULL
				  AND (oidc_issuer IS NULL OR BTRIM(oidc_issuer) = '')`,
		},
	}
}

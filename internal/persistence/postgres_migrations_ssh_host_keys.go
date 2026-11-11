package persistence

// schemaMigrationSSHHostKeyPins stores SSH host keys people trusted on first
// connect: Quick connect keys in ssh_known_hosts (one per host and port), a
// bookmark's key on the bookmark with the host and port it was trusted for.
func schemaMigrationSSHHostKeyPins() schemaMigration {
	return schemaMigration{Version: 112, Name: "ssh_host_key_pins", Statements: []string{
		`CREATE TABLE IF NOT EXISTS ssh_known_hosts (
			host TEXT NOT NULL,
			port INTEGER NOT NULL CHECK (port BETWEEN 1 AND 65535),
			host_key TEXT NOT NULL,
			fingerprint TEXT NOT NULL,
			pinned_by TEXT NOT NULL DEFAULT '',
			pinned_at TIMESTAMPTZ NOT NULL,
			PRIMARY KEY (host, port)
		)`,
		`ALTER TABLE terminal_session_bookmarks ADD COLUMN IF NOT EXISTS host_key TEXT`,
		`ALTER TABLE terminal_session_bookmarks ADD COLUMN IF NOT EXISTS host_key_target TEXT`,
	}}
}

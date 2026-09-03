package persistence

import (
	"strings"
	"testing"
)

func TestRemoteBookmarkSPICESecurityUsesAppendOnlyMigration(t *testing.T) {
	const version = 100
	for _, migration := range postgresSchemaMigrations() {
		if migration.Version != version {
			continue
		}
		joined := strings.Join(migration.Statements, "\n")
		for _, required := range []string{
			"spice_security_mode TEXT NOT NULL DEFAULT 'tls'",
			"spice_ca_pem TEXT NOT NULL DEFAULT ''",
			"CHECK (spice_security_mode IN ('tls', 'cleartext'))",
		} {
			if !strings.Contains(joined, required) {
				t.Fatalf("migration %d missing %q", version, required)
			}
		}
		return
	}
	t.Fatalf("migration %d not found", version)
}

package persistence

import (
	"strings"
	"testing"
)

func TestCollectorURLUserinfoMigrationScrubsCopiesAndDisablesCollectors(t *testing.T) {
	const version = 99
	for _, migration := range postgresSchemaMigrations() {
		if migration.Version != version {
			continue
		}
		statements := strings.Join(migration.Statements, "\n")
		for _, required := range []string{
			"CREATE FUNCTION labtether_migration_99_scrub_url_userinfo",
			"UPDATE hub_collectors",
			"enabled = FALSE",
			"last_error = regexp_replace",
			"[^/?#[:cntrl:]]*@",
			"credential_profiles",
			"UPDATE assets",
			"asset_heartbeats",
			"provider_instances",
			"resource_external_refs",
			"DROP FUNCTION labtether_migration_99_scrub_url_userinfo",
		} {
			if !strings.Contains(statements, required) {
				t.Fatalf("migration v%d is missing %q", version, required)
			}
		}
		if strings.Contains(statements, "credential-bearing-secret") {
			t.Fatal("migration contains test credential material")
		}
		return
	}
	t.Fatalf("missing collector URL userinfo migration v%d", version)
}

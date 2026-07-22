package persistence

import (
	"strings"
	"testing"
)

func TestSyntheticHubMetricScopeUsesAppendOnlyMigration(t *testing.T) {
	const version = 96
	for _, migration := range postgresSchemaMigrations() {
		if migration.Version != version {
			continue
		}
		statements := strings.Join(migration.Statements, "\n")
		for _, required := range []string{
			"DROP CONSTRAINT IF EXISTS hub_metric_samples_scope_check",
			"ADD CONSTRAINT hub_metric_samples_scope_check",
			"scope IN ('hub-alerts', 'hub-reliability', 'hub-synthetic')",
		} {
			if !strings.Contains(statements, required) {
				t.Fatalf("migration v%d is missing %q: %s", version, required, statements)
			}
		}
		return
	}
	t.Fatalf("missing synthetic hub metric scope migration v%d", version)
}

package persistence

import "testing"

func TestPostgresAdminResetTableCountAndHubMetricsStayInSync(t *testing.T) {
	tables := postgresAdminResetTables()
	if len(tables) != 53 {
		t.Fatalf("admin reset table count = %d, want 53", len(tables))
	}
	seen := make(map[string]struct{}, len(tables))
	for _, table := range tables {
		if _, duplicate := seen[table]; duplicate {
			t.Fatalf("admin reset table list contains duplicate %q", table)
		}
		seen[table] = struct{}{}
	}
	if _, ok := seen["hub_metric_samples"]; !ok {
		t.Fatal("admin reset omits hub_metric_samples")
	}
	// The audit trail must survive a reset so an admin cannot erase it.
	if _, ok := seen["audit_events"]; ok {
		t.Fatal("admin reset truncates audit_events")
	}
}

package persistence

import (
	"regexp"
	"strings"
	"testing"
)

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

// One TRUNCATE clears every reset table, so a single name that is a view or
// no longer exists fails the whole reset. Migration 60 renamed
// asset_dependencies to asset_edges and left a view, and Reset hub failed
// with "is not a table" until the list followed the rename.
func TestPostgresAdminResetNamesOnlyTablesTheMigrationsLeave(t *testing.T) {
	event := regexp.MustCompile(`(?i)\b(?:CREATE TABLE(?: IF NOT EXISTS)?\s+(\w+)|ALTER TABLE(?: IF EXISTS)?\s+(\w+)\s+RENAME TO\s+(\w+)|CREATE(?: OR REPLACE)? VIEW\s+(\w+)|DROP TABLE(?: IF EXISTS)?\s+(\w+))`)
	kind := map[string]string{}
	for _, migration := range postgresSchemaMigrations() {
		for _, statement := range migration.Statements {
			for _, m := range event.FindAllStringSubmatch(statement, -1) {
				switch {
				case m[1] != "":
					kind[strings.ToLower(m[1])] = "table"
				case m[2] != "":
					kind[strings.ToLower(m[2])] = "renamed to " + m[3]
					kind[strings.ToLower(m[3])] = "table"
				case m[4] != "":
					kind[strings.ToLower(m[4])] = "view"
				case m[5] != "":
					kind[strings.ToLower(m[5])] = "dropped"
				}
			}
		}
	}
	for _, table := range postgresAdminResetTables() {
		if got := kind[table]; got != "table" {
			if got == "" {
				got = "never created"
			}
			t.Errorf("admin reset truncates %q, which the migrations leave as: %s", table, got)
		}
	}
}

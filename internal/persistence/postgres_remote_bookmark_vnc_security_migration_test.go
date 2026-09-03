package persistence

import (
	"strings"
	"testing"
)

func TestRemoteBookmarkVNCSecurityUsesAppendOnlyMigration(t *testing.T) {
	migrations := postgresSchemaMigrations()
	for _, migration := range migrations {
		if migration.Version != 101 {
			continue
		}
		joined := strings.Join(migration.Statements, "\n")
		if !strings.Contains(joined, "allow_insecure_vnc BOOLEAN NOT NULL DEFAULT false") {
			t.Fatalf("migration 101 does not default saved VNC bookmarks to blocked: %s", joined)
		}
		return
	}
	t.Fatal("migration 101 was not found")
}

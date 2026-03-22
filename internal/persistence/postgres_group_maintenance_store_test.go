package persistence

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/groups"
)

func TestCreateGroup_AllowsBlankTimezoneAndLocation(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if strings.TrimSpace(dbURL) == "" {
		t.Skip("DATABASE_URL not set, skipping persistence integration test")
	}

	store, err := NewPostgresStore(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("NewPostgresStore failed: %v", err)
	}
	defer store.Close()

	group := createIntegrationGroupWithCleanup(t, store, groups.CreateRequest{
		Name:     "Blank Optional Fields Group",
		Slug:     "blank-optional-fields-group-" + strconv.FormatInt(time.Now().UTC().UnixNano(), 10),
		Timezone: "   ",
		Location: "   ",
	})

	if group.Timezone != "" {
		t.Fatalf("expected blank timezone to round-trip as empty string, got %q", group.Timezone)
	}
	if group.Location != "" {
		t.Fatalf("expected blank location to round-trip as empty string, got %q", group.Location)
	}
}

func createIntegrationGroupWithCleanup(t *testing.T, store *PostgresStore, req groups.CreateRequest) groups.Group {
	t.Helper()

	group, err := store.CreateGroup(req)
	if err != nil {
		t.Fatalf("CreateGroup(%s) failed: %v", strings.TrimSpace(req.Slug), err)
	}

	t.Cleanup(func() {
		_ = store.DeleteGroup(group.ID)
	})

	return group
}

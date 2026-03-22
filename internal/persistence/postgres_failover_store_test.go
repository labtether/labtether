package persistence

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/groupfailover"
	"github.com/labtether/labtether/internal/groups"
)

func TestCreateFailoverPair_DefaultsNameToEmptyString(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if strings.TrimSpace(dbURL) == "" {
		t.Skip("DATABASE_URL not set, skipping persistence integration test")
	}

	store, err := NewPostgresStore(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("NewPostgresStore failed: %v", err)
	}
	defer store.Close()

	ts := strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
	primaryGroup := createIntegrationGroupWithCleanup(t, store, groups.CreateRequest{
		Name: "Test Failover Primary " + ts,
		Slug: "tfp-" + ts,
	})
	backupGroup := createIntegrationGroupWithCleanup(t, store, groups.CreateRequest{
		Name: "Test Failover Backup " + ts,
		Slug: "tfb-" + ts,
	})

	pair, err := store.CreateFailoverPair(groupfailover.CreatePairRequest{
		PrimaryGroupID: primaryGroup.ID,
		BackupGroupID:  backupGroup.ID,
		Name:           "   ",
	})
	if err != nil {
		t.Fatalf("CreateFailoverPair failed with blank name: %v", err)
	}

	if pair.Name != "" {
		t.Fatalf("expected empty string name when omitted, got %q", pair.Name)
	}
}

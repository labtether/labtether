package persistence

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/credentials"
)

func TestMemoryCredentialProfileOwnerLimitIsAtomic(t *testing.T) {
	store := NewMemoryCredentialStore()
	const limit = 7

	start := make(chan struct{})
	results := make(chan error, 32)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			_, err := store.CreateCredentialProfileBounded(credentials.Profile{
				ID:   fmt.Sprintf("cred_concurrent_%d", index),
				Name: "Concurrent",
				Kind: credentials.KindSSHPassword,
			}, "owner-a", limit, 100)
			results <- err
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)

	created := 0
	limited := 0
	for err := range results {
		switch {
		case err == nil:
			created++
		case errors.Is(err, ErrCredentialProfileOwnerLimit):
			limited++
		default:
			t.Fatalf("unexpected create error: %v", err)
		}
	}
	if created != limit || limited != 32-limit {
		t.Fatalf("created=%d limited=%d", created, limited)
	}
}

func TestPostgresCredentialProfileOwnerLimitIsAtomic(t *testing.T) {
	store := newTestPostgresStore(t)
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	owner := "owner_atomic_" + suffix
	const limit = 5

	start := make(chan struct{})
	results := make(chan error, 16)
	createdIDs := make(chan string, 16)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			profile, err := store.CreateCredentialProfileBounded(credentials.Profile{
				ID:               fmt.Sprintf("cred_pg_atomic_%s_%d", suffix, index),
				Name:             "Postgres concurrent",
				Kind:             credentials.KindSSHPassword,
				SecretCiphertext: "ciphertext",
			}, owner, limit, 100_000)
			if err == nil {
				createdIDs <- profile.ID
			}
			results <- err
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)
	close(createdIDs)
	defer func() {
		for id := range createdIDs {
			_ = store.DeleteCredentialProfile(id)
		}
	}()

	created := 0
	for err := range results {
		if err == nil {
			created++
			continue
		}
		if !errors.Is(err, ErrCredentialProfileOwnerLimit) {
			t.Fatalf("unexpected create error: %v", err)
		}
	}
	if created != limit {
		t.Fatalf("created=%d want=%d", created, limit)
	}
}

func TestPostgresCredentialProfileDeleteDetectsBookmarkReference(t *testing.T) {
	store := newTestPostgresStore(t)
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	profileID := "cred_pg_reference_" + suffix
	bookmarkID := "bookmark_pg_reference_" + suffix
	if _, err := store.CreateCredentialProfileBounded(credentials.Profile{
		ID:               profileID,
		Name:             "Referenced",
		Kind:             credentials.KindSSHPassword,
		SecretCiphertext: "ciphertext",
	}, "owner_reference_"+suffix, 100, 100_000); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.DeleteCredentialProfile(profileID) }()
	if _, err := store.pool.Exec(t.Context(),
		`INSERT INTO terminal_session_bookmarks (id, actor_id, title, credential_profile_id) VALUES ($1, $2, $3, $4)`,
		bookmarkID, "actor_"+suffix, "Referenced", profileID,
	); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = store.pool.Exec(t.Context(), `DELETE FROM terminal_session_bookmarks WHERE id = $1`, bookmarkID)
	}()

	summary, err := store.DeleteCredentialProfileIfUnreferenced(profileID)
	if !errors.Is(err, ErrCredentialProfileInUse) {
		t.Fatalf("delete error=%v summary=%+v", err, summary)
	}
	if summary.Total != 1 || len(summary.References) != 1 || summary.References[0].Resource != "terminal_session_bookmarks" {
		t.Fatalf("unexpected reference summary: %+v", summary)
	}
	if _, err = store.pool.Exec(t.Context(), `DELETE FROM terminal_session_bookmarks WHERE id = $1`, bookmarkID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.DeleteCredentialProfileIfUnreferenced(profileID); err != nil {
		t.Fatalf("delete after unlink: %v", err)
	}
}

func TestCredentialProfileLifecycleMigrationAddsCreatorAndReferenceIndexes(t *testing.T) {
	for _, migration := range postgresSchemaMigrations() {
		if migration.Version != 95 {
			continue
		}
		joined := ""
		for _, statement := range migration.Statements {
			joined += statement + "\n"
		}
		for _, required := range []string{
			"credential_profiles ADD COLUMN IF NOT EXISTS created_by",
			"idx_credential_profiles_created_by",
			"idx_asset_protocol_configs_credential",
			"idx_terminal_session_bookmarks_credential",
			"idx_hub_collectors_credential",
			"idx_groups_jump_chain_gin",
		} {
			if !strings.Contains(joined, required) {
				t.Fatalf("migration 95 missing %q", required)
			}
		}
		return
	}
	t.Fatal("missing credential lifecycle migration 95")
}

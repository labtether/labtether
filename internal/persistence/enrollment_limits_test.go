package persistence

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/enrollment"
)

func TestMemoryEnrollmentStoreRejectsOutOfBoundsStoredMaxUses(t *testing.T) {
	store := NewMemoryEnrollmentStore()
	token := enrollment.EnrollmentToken{
		ID:        "legacy-oversized-max-uses",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
		MaxUses:   enrollment.HardTokenMaxUsesCeiling + 1,
		CreatedAt: time.Now().UTC(),
	}
	store.mu.Lock()
	store.enrollmentTokens[token.ID] = token
	store.enrollmentByHash["legacy-oversized-hash"] = token.ID
	store.mu.Unlock()

	if _, valid, err := store.ValidateEnrollmentToken("legacy-oversized-hash"); err != nil || valid {
		t.Fatalf("oversized legacy token valid=%v err=%v", valid, err)
	}
	if _, consumed, err := store.ConsumeEnrollmentToken("legacy-oversized-hash"); err != nil || consumed {
		t.Fatalf("oversized legacy token consumed=%v err=%v", consumed, err)
	}
	if err := store.IncrementEnrollmentTokenUse(token.ID); !errors.Is(err, ErrEnrollmentTokenInvalid) {
		t.Fatalf("oversized legacy token increment error=%v", err)
	}
	deleted, _, err := store.DeleteDeadTokens()
	if err != nil || deleted != 1 {
		t.Fatalf("oversized legacy token cleanup deleted=%d err=%v", deleted, err)
	}
}

func TestPostgresEnrollmentStoreRejectsOutOfBoundsStoredMaxUses(t *testing.T) {
	store := newTestPostgresStore(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	tokenID := "legacy-oversized-" + suffix
	tokenHash := "legacy-oversized-hash-" + suffix
	t.Cleanup(func() {
		_, _ = store.pool.Exec(ctx, `DELETE FROM enrollment_tokens WHERE id = $1`, tokenID)
	})
	if _, err := store.pool.Exec(ctx,
		`INSERT INTO enrollment_tokens (id, token_hash, label, expires_at, max_uses, use_count, created_at)
		 VALUES ($1, $2, 'legacy oversized', clock_timestamp() + interval '1 hour', $3, 0, clock_timestamp())`,
		tokenID, tokenHash, enrollment.HardTokenMaxUsesCeiling+1,
	); err != nil {
		t.Fatal(err)
	}

	if _, valid, err := store.ValidateEnrollmentToken(tokenHash); err != nil || valid {
		t.Fatalf("oversized legacy PG token valid=%v err=%v", valid, err)
	}
	if _, consumed, err := store.ConsumeEnrollmentToken(tokenHash); err != nil || consumed {
		t.Fatalf("oversized legacy PG token consumed=%v err=%v", consumed, err)
	}
	if err := store.IncrementEnrollmentTokenUse(tokenID); !errors.Is(err, ErrEnrollmentTokenInvalid) {
		t.Fatalf("oversized legacy PG token increment error=%v", err)
	}
}

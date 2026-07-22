package persistence

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/auth"
)

func TestPostgresRotateAgentTokenSerializesConcurrentReplacements(t *testing.T) {
	store := newTestPostgresStore(t)
	assetID := fmt.Sprintf("token-rotation-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		_, _ = store.pool.Exec(context.Background(), `DELETE FROM agent_tokens WHERE asset_id = $1`, assetID)
	})

	_, oldHash, err := auth.GenerateSessionToken()
	if err != nil {
		t.Fatalf("generate old token: %v", err)
	}
	if _, err := store.CreateAgentToken(assetID, oldHash, "initial", time.Now().UTC().Add(time.Hour)); err != nil {
		t.Fatalf("create old token: %v", err)
	}

	replacementHashes := make([]string, 2)
	for index := range replacementHashes {
		_, replacementHashes[index], err = auth.GenerateSessionToken()
		if err != nil {
			t.Fatalf("generate replacement token %d: %v", index, err)
		}
	}

	start := make(chan struct{})
	errCh := make(chan error, len(replacementHashes))
	var wg sync.WaitGroup
	for index, hash := range replacementHashes {
		wg.Add(1)
		go func(index int, hash string) {
			defer wg.Done()
			<-start
			_, rotateErr := store.RotateAgentToken(assetID, hash, fmt.Sprintf("replacement-%d", index), time.Now().UTC().Add(time.Hour))
			errCh <- rotateErr
		}(index, hash)
	}
	close(start)
	wg.Wait()
	close(errCh)
	for rotateErr := range errCh {
		if rotateErr != nil {
			t.Fatalf("rotate token: %v", rotateErr)
		}
	}

	var activeCount int
	if err := store.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM agent_tokens WHERE asset_id = $1 AND status = 'active'`, assetID,
	).Scan(&activeCount); err != nil {
		t.Fatalf("count active tokens: %v", err)
	}
	if activeCount != 1 {
		t.Fatalf("active token count=%d, want 1", activeCount)
	}
	if _, valid, err := store.ValidateAgentToken(oldHash); err != nil || valid {
		t.Fatalf("old token valid=%v err=%v", valid, err)
	}
	replacementValidCount := 0
	for _, hash := range replacementHashes {
		if _, valid, err := store.ValidateAgentToken(hash); err != nil {
			t.Fatalf("validate replacement token: %v", err)
		} else if valid {
			replacementValidCount++
		}
	}
	if replacementValidCount != 1 {
		t.Fatalf("valid replacement token count=%d, want 1", replacementValidCount)
	}
}

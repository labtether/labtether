package persistence

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/savedactions"
)

func TestMemorySavedActionCapacityIsAtomicPerActor(t *testing.T) {
	store := NewMemorySavedActionStore()
	ctx := context.Background()
	actorID := "actor-capacity"
	const contenders = savedactions.MaxActionsPerActor + 75
	start := make(chan struct{})
	var wg sync.WaitGroup
	var successCount int
	var capacityCount int
	var resultMu sync.Mutex
	var unexpected []error
	for index := range contenders {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			err := store.CreateSavedAction(ctx, savedactions.SavedAction{
				ID:        fmt.Sprintf("act-%d", index),
				Name:      "capacity",
				CreatedBy: actorID,
				CreatedAt: time.Unix(int64(index), 0).UTC(),
			})
			resultMu.Lock()
			defer resultMu.Unlock()
			switch {
			case err == nil:
				successCount++
			case errors.Is(err, savedactions.ErrCapacity):
				capacityCount++
			default:
				unexpected = append(unexpected, err)
			}
		}()
	}
	close(start)
	wg.Wait()
	if len(unexpected) != 0 {
		t.Fatalf("unexpected create errors: %v", unexpected)
	}
	if successCount != savedactions.MaxActionsPerActor || capacityCount != contenders-savedactions.MaxActionsPerActor {
		t.Fatalf("success=%d capacity=%d, want %d/%d", successCount, capacityCount, savedactions.MaxActionsPerActor, contenders-savedactions.MaxActionsPerActor)
	}
	list, total, err := store.ListSavedActions(ctx, actorID, savedactions.MaxActionsPerActor, 0)
	if err != nil || total != savedactions.MaxActionsPerActor || len(list) != savedactions.MaxActionsPerActor {
		t.Fatalf("stored actions total=%d len=%d err=%v", total, len(list), err)
	}
	if err := store.CreateSavedAction(ctx, savedactions.SavedAction{ID: "other-actor", Name: "other", CreatedBy: "actor-other", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("capacity leaked across actors: %v", err)
	}
}

func TestPostgresSavedActionCapacitySerializesConcurrentCreators(t *testing.T) {
	store := newTestPostgresStore(t)
	ctx := context.Background()
	actorID := idgen.New("saved-action-capacity-actor")
	prefix := idgen.New("saved-action-capacity-seed")
	t.Cleanup(func() {
		_, _ = store.pool.Exec(context.Background(), `DELETE FROM saved_actions WHERE created_by = $1`, actorID)
	})
	if _, err := store.pool.Exec(ctx, `
		INSERT INTO saved_actions (id, name, description, steps, created_by, created_at)
		SELECT $1 || '-' || value::text, 'capacity seed', '', '[]'::jsonb, $2, NOW()
		FROM generate_series(1, $3) AS value`, prefix, actorID, savedactions.MaxActionsPerActor-1); err != nil {
		t.Fatalf("seed saved actions: %v", err)
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	for index := range 2 {
		go func() {
			<-start
			results <- store.CreateSavedAction(ctx, savedactions.SavedAction{
				ID:        fmt.Sprintf("%s-contender-%d", prefix, index),
				Name:      "capacity contender",
				CreatedBy: actorID,
				CreatedAt: time.Now().UTC(),
			})
		}()
	}
	close(start)
	first, second := <-results, <-results
	successes, capacities := 0, 0
	for _, err := range []error{first, second} {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, savedactions.ErrCapacity):
			capacities++
		default:
			t.Fatalf("unexpected create error: %v", err)
		}
	}
	if successes != 1 || capacities != 1 {
		t.Fatalf("successes=%d capacities=%d, want 1/1", successes, capacities)
	}
	var count int
	if err := store.pool.QueryRow(ctx, `SELECT COUNT(*) FROM saved_actions WHERE created_by = $1`, actorID).Scan(&count); err != nil {
		t.Fatalf("count saved actions: %v", err)
	}
	if count != savedactions.MaxActionsPerActor {
		t.Fatalf("saved action count=%d, want %d", count, savedactions.MaxActionsPerActor)
	}
}

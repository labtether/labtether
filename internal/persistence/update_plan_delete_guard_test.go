package persistence

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/updates"
)

func TestMemoryUpdatePlanDeleteRejectsActiveRunsAndCascadesTerminalRuns(t *testing.T) {
	for _, status := range []string{updates.StatusQueued, updates.StatusRunning} {
		t.Run("rejects_"+status, func(t *testing.T) {
			store := NewMemoryUpdateStore()
			plan := mustCreateMemoryUpdatePlan(t, store, "active "+status)
			run, err := store.CreateUpdateRun(plan, updates.ExecutePlanRequest{ActorID: "qa"})
			if err != nil {
				t.Fatalf("create update run: %v", err)
			}
			if status == updates.StatusRunning {
				store.mu.Lock()
				stored := store.runs[run.ID]
				stored.Status = updates.StatusRunning
				store.runs[run.ID] = stored
				store.mu.Unlock()
			}

			if err := store.DeleteUpdatePlan(plan.ID); !errors.Is(err, ErrUpdatePlanActive) {
				t.Fatalf("delete active plan error=%v, want ErrUpdatePlanActive", err)
			}
			if _, ok, err := store.GetUpdatePlan(plan.ID); err != nil || !ok {
				t.Fatalf("active plan was removed: ok=%t err=%v", ok, err)
			}
			if _, ok, err := store.GetUpdateRun(run.ID); err != nil || !ok {
				t.Fatalf("active run was removed: ok=%t err=%v", ok, err)
			}
		})
	}

	store := NewMemoryUpdateStore()
	deletedPlan := mustCreateMemoryUpdatePlan(t, store, "terminal")
	terminalRun, err := store.CreateUpdateRun(deletedPlan, updates.ExecutePlanRequest{ActorID: "qa"})
	if err != nil {
		t.Fatalf("create terminal run: %v", err)
	}
	if err := store.ApplyUpdateResult(updates.Result{
		RunID:       terminalRun.ID,
		Status:      updates.StatusSucceeded,
		CompletedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("complete terminal run: %v", err)
	}
	unrelatedPlan := mustCreateMemoryUpdatePlan(t, store, "unrelated")
	unrelatedRun, err := store.CreateUpdateRun(unrelatedPlan, updates.ExecutePlanRequest{ActorID: "qa"})
	if err != nil {
		t.Fatalf("create unrelated run: %v", err)
	}

	if err := store.DeleteUpdatePlan(deletedPlan.ID); err != nil {
		t.Fatalf("delete terminal plan: %v", err)
	}
	if _, ok, err := store.GetUpdatePlan(deletedPlan.ID); err != nil || ok {
		t.Fatalf("deleted plan remains: ok=%t err=%v", ok, err)
	}
	if _, ok, err := store.GetUpdateRun(terminalRun.ID); err != nil || ok {
		t.Fatalf("terminal run was orphaned: ok=%t err=%v", ok, err)
	}
	if _, ok, err := store.GetUpdatePlan(unrelatedPlan.ID); err != nil || !ok {
		t.Fatalf("unrelated plan was removed: ok=%t err=%v", ok, err)
	}
	if _, ok, err := store.GetUpdateRun(unrelatedRun.ID); err != nil || !ok {
		t.Fatalf("unrelated run was removed: ok=%t err=%v", ok, err)
	}
}

func TestMemoryUpdatePlanDeleteAndRunCreateAreAtomic(t *testing.T) {
	for iteration := 0; iteration < 100; iteration++ {
		store := NewMemoryUpdateStore()
		plan := mustCreateMemoryUpdatePlan(t, store, fmt.Sprintf("race-%d", iteration))
		start := make(chan struct{})
		var wg sync.WaitGroup
		var run updates.Run
		var createErr, deleteErr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			run, createErr = store.CreateUpdateRun(plan, updates.ExecutePlanRequest{ActorID: "qa"})
		}()
		go func() {
			defer wg.Done()
			<-start
			deleteErr = store.DeleteUpdatePlan(plan.ID)
		}()
		close(start)
		wg.Wait()

		switch {
		case createErr == nil && errors.Is(deleteErr, ErrUpdatePlanActive):
			if _, ok, err := store.GetUpdatePlan(plan.ID); err != nil || !ok {
				t.Fatalf("iteration %d: active plan missing: ok=%t err=%v", iteration, ok, err)
			}
			if _, ok, err := store.GetUpdateRun(run.ID); err != nil || !ok {
				t.Fatalf("iteration %d: active run missing: ok=%t err=%v", iteration, ok, err)
			}
		case errors.Is(createErr, ErrNotFound) && deleteErr == nil:
			if runs, err := store.ListUpdateRuns(10, ""); err != nil || len(runs) != 0 {
				t.Fatalf("iteration %d: orphan runs=%d err=%v", iteration, len(runs), err)
			}
		default:
			t.Fatalf("iteration %d: createErr=%v deleteErr=%v", iteration, createErr, deleteErr)
		}
	}
}

func TestPostgresUpdatePlanDeleteRejectsActiveRunsAndCascadesTerminalRuns(t *testing.T) {
	store := newTestPostgresStore(t)
	ctx := context.Background()
	suffix := time.Now().UTC().UnixNano()
	plan, err := store.CreateUpdatePlan(updates.CreatePlanRequest{
		Name:    fmt.Sprintf("delete-guard-%d", suffix),
		Targets: []string{fmt.Sprintf("delete-guard-target-%d", suffix)},
	})
	if err != nil {
		t.Fatalf("create update plan: %v", err)
	}
	t.Cleanup(func() {
		_, _ = store.pool.Exec(ctx, `DELETE FROM update_plans WHERE id = $1`, plan.ID)
	})
	run, err := store.CreateUpdateRun(plan, updates.ExecutePlanRequest{ActorID: "qa"})
	if err != nil {
		t.Fatalf("create update run: %v", err)
	}

	if err := store.DeleteUpdatePlan(plan.ID); !errors.Is(err, ErrUpdatePlanActive) {
		t.Fatalf("delete queued plan error=%v, want ErrUpdatePlanActive", err)
	}
	if _, ok, err := store.GetUpdatePlan(plan.ID); err != nil || !ok {
		t.Fatalf("queued plan was removed: ok=%t err=%v", ok, err)
	}
	if _, ok, err := store.GetUpdateRun(run.ID); err != nil || !ok {
		t.Fatalf("queued run was removed: ok=%t err=%v", ok, err)
	}

	if _, err := store.pool.Exec(ctx,
		`UPDATE update_runs SET status = $2, updated_at = NOW() WHERE id = $1`,
		run.ID,
		updates.StatusRunning,
	); err != nil {
		t.Fatalf("mark run running: %v", err)
	}
	if err := store.DeleteUpdatePlan(plan.ID); !errors.Is(err, ErrUpdatePlanActive) {
		t.Fatalf("delete running plan error=%v, want ErrUpdatePlanActive", err)
	}

	if err := store.ApplyUpdateResult(updates.Result{
		RunID:       run.ID,
		Status:      updates.StatusSucceeded,
		CompletedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("complete update run: %v", err)
	}
	if err := store.DeleteUpdatePlan(plan.ID); err != nil {
		t.Fatalf("delete terminal plan: %v", err)
	}
	if _, ok, err := store.GetUpdatePlan(plan.ID); err != nil || ok {
		t.Fatalf("deleted plan remains: ok=%t err=%v", ok, err)
	}
	if _, ok, err := store.GetUpdateRun(run.ID); err != nil || ok {
		t.Fatalf("terminal run was orphaned: ok=%t err=%v", ok, err)
	}
}

func TestPostgresUpdatePlanDeleteAndRunCreateAreAtomic(t *testing.T) {
	store := newTestPostgresStore(t)
	for iteration := 0; iteration < 25; iteration++ {
		plan, err := store.CreateUpdatePlan(updates.CreatePlanRequest{
			Name:    fmt.Sprintf("postgres-delete-race-%d-%d", time.Now().UTC().UnixNano(), iteration),
			Targets: []string{fmt.Sprintf("postgres-delete-race-target-%d", iteration)},
		})
		if err != nil {
			t.Fatalf("iteration %d: create update plan: %v", iteration, err)
		}
		start := make(chan struct{})
		var wg sync.WaitGroup
		var run updates.Run
		var createErr, deleteErr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			run, createErr = store.CreateUpdateRun(plan, updates.ExecutePlanRequest{ActorID: "qa"})
		}()
		go func() {
			defer wg.Done()
			<-start
			deleteErr = store.DeleteUpdatePlan(plan.ID)
		}()
		close(start)
		wg.Wait()

		switch {
		case createErr == nil && errors.Is(deleteErr, ErrUpdatePlanActive):
			if _, ok, err := store.GetUpdatePlan(plan.ID); err != nil || !ok {
				t.Fatalf("iteration %d: active plan missing: ok=%t err=%v", iteration, ok, err)
			}
			if _, ok, err := store.GetUpdateRun(run.ID); err != nil || !ok {
				t.Fatalf("iteration %d: active run missing: ok=%t err=%v", iteration, ok, err)
			}
			if err := store.ApplyUpdateResult(updates.Result{
				RunID:       run.ID,
				Status:      updates.StatusSucceeded,
				CompletedAt: time.Now().UTC(),
			}); err != nil {
				t.Fatalf("iteration %d: complete update run: %v", iteration, err)
			}
			if err := store.DeleteUpdatePlan(plan.ID); err != nil {
				t.Fatalf("iteration %d: cleanup completed plan: %v", iteration, err)
			}
		case errors.Is(createErr, ErrNotFound) && deleteErr == nil:
			if _, ok, err := store.GetUpdatePlan(plan.ID); err != nil || ok {
				t.Fatalf("iteration %d: delete winner left plan: ok=%t err=%v", iteration, ok, err)
			}
		default:
			t.Fatalf("iteration %d: createErr=%v deleteErr=%v", iteration, createErr, deleteErr)
		}
	}
}

func mustCreateMemoryUpdatePlan(t *testing.T, store *MemoryUpdateStore, name string) updates.Plan {
	t.Helper()
	plan, err := store.CreateUpdatePlan(updates.CreatePlanRequest{
		Name:    name,
		Targets: []string{"asset-qa"},
	})
	if err != nil {
		t.Fatalf("create update plan: %v", err)
	}
	return plan
}

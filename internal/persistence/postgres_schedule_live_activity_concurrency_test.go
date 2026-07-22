package persistence

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/schedules"
)

func TestPostgresScheduledTaskPrincipalCapacityIsAtomic(t *testing.T) {
	store := newTestPostgresStore(t)
	ctx := context.Background()
	suffix := time.Now().UTC().UnixNano()
	principalID := fmt.Sprintf("schedule-capacity-%d", suffix)
	idPrefix := fmt.Sprintf("schedule-capacity-%d-", suffix)
	t.Cleanup(func() {
		_, _ = store.pool.Exec(context.Background(), `DELETE FROM scheduled_tasks WHERE created_by = $1`, principalID)
	})

	_, err := store.pool.Exec(ctx, `
		INSERT INTO scheduled_tasks (id, name, cron_expr, command, targets, enabled, created_by, created_at)
		SELECT $1 || n::text, 'capacity fixture', '0 * * * *', 'true', '[]'::jsonb, FALSE, $2, NOW()
		  FROM generate_series(1, $3) AS n
	`, idPrefix, principalID, schedules.MaxScheduledTasksPerPrincipal-1)
	if err != nil {
		t.Fatalf("seed scheduled-task capacity fixture: %v", err)
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			results <- store.CreateScheduledTask(ctx, schedules.ScheduledTask{
				ID:        fmt.Sprintf("%scontender-%d", idPrefix, index),
				Name:      "capacity contender",
				CronExpr:  "0 * * * *",
				Command:   "true",
				Targets:   []string{},
				Enabled:   false,
				CreatedBy: principalID,
				CreatedAt: time.Now().UTC(),
			})
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)

	var succeeded, rejected int
	for err := range results {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, schedules.ErrScheduledTaskCapacityExceeded):
			rejected++
		default:
			t.Fatalf("unexpected concurrent create result: %v", err)
		}
	}
	if succeeded != 1 || rejected != 1 {
		t.Fatalf("concurrent creates succeeded=%d rejected=%d, want 1/1", succeeded, rejected)
	}

	var count int
	if err := store.pool.QueryRow(ctx, `SELECT COUNT(*) FROM scheduled_tasks WHERE created_by = $1`, principalID).Scan(&count); err != nil {
		t.Fatalf("count capacity fixture: %v", err)
	}
	if count != schedules.MaxScheduledTasksPerPrincipal {
		t.Fatalf("principal schedule count=%d, want %d", count, schedules.MaxScheduledTasksPerPrincipal)
	}
}

func TestPostgresScheduledTaskOccurrenceClaimIsSingleWinner(t *testing.T) {
	store := newTestPostgresStore(t)
	ctx := context.Background()
	suffix := time.Now().UTC().UnixNano()
	scheduleID := fmt.Sprintf("schedule-claim-%d", suffix)
	jobID := fmt.Sprintf("schedule-job-%d", suffix)
	dueAt := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	claimedAt := time.Now().UTC().Truncate(time.Microsecond)
	nextRunAt := claimedAt.Add(time.Hour)
	t.Cleanup(func() {
		_, _ = store.pool.Exec(context.Background(), `DELETE FROM job_queue WHERE id = $1`, jobID)
		_, _ = store.pool.Exec(context.Background(), `DELETE FROM scheduled_tasks WHERE id = $1`, scheduleID)
	})

	if err := store.CreateScheduledTask(ctx, schedules.ScheduledTask{
		ID:        scheduleID,
		Name:      "claim fixture",
		CronExpr:  "0 * * * *",
		Command:   "true",
		Targets:   []string{"asset-qa"},
		Enabled:   true,
		CreatedBy: fmt.Sprintf("schedule-claim-owner-%d", suffix),
		CreatedAt: claimedAt,
		NextRunAt: &dueAt,
	}); err != nil {
		t.Fatalf("create claim fixture: %v", err)
	}

	claim := schedules.ExecutionClaim{
		ScheduleID:  scheduleID,
		DueAt:       dueAt,
		ClaimedAt:   claimedAt,
		NextRunAt:   nextRunAt,
		MaxAttempts: 3,
		Job: schedules.ExecutionJob{
			JobID:        jobID,
			ScheduleID:   scheduleID,
			ScheduleName: "claim fixture",
			ScheduledFor: dueAt,
			Command:      "true",
			Targets:      []string{"asset-qa"},
			ActorID:      "schedule-claim-owner",
		},
	}

	start := make(chan struct{})
	results := make(chan struct {
		claimed bool
		err     error
	}, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			claimed, err := store.ClaimScheduledTaskExecution(ctx, claim)
			results <- struct {
				claimed bool
				err     error
			}{claimed: claimed, err: err}
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	var winners int
	for result := range results {
		if result.err != nil {
			t.Fatalf("claim scheduled occurrence: %v", result.err)
		}
		if result.claimed {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("claim winners=%d, want 1", winners)
	}

	var jobs int
	if err := store.pool.QueryRow(ctx, `SELECT COUNT(*) FROM job_queue WHERE id = $1`, jobID).Scan(&jobs); err != nil {
		t.Fatalf("count claimed queue jobs: %v", err)
	}
	if jobs != 1 {
		t.Fatalf("queue jobs=%d, want 1", jobs)
	}

	task, ok, err := store.GetScheduledTask(ctx, scheduleID)
	if err != nil || !ok {
		t.Fatalf("load claimed schedule: ok=%t err=%v", ok, err)
	}
	if task.LastRunStatus != "queued" || task.LastRunJobID != jobID || task.NextRunAt == nil || !task.NextRunAt.Equal(nextRunAt) {
		t.Fatalf("claimed schedule state=%+v, want queued job %q next %s", task, jobID, nextRunAt)
	}
}

func TestPostgresScheduledTaskEvaluationIndexMatchesNullFirstOrder(t *testing.T) {
	store := newTestPostgresStore(t)
	ctx := context.Background()

	tx, err := store.pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin query-plan transaction: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SET LOCAL enable_seqscan = off`); err != nil {
		t.Fatalf("disable sequential scan for deterministic plan proof: %v", err)
	}
	if _, err := tx.Exec(ctx, `SET LOCAL enable_bitmapscan = off`); err != nil {
		t.Fatalf("disable bitmap scan for deterministic ordered-index proof: %v", err)
	}
	rows, err := tx.Query(ctx, `
		EXPLAIN (COSTS OFF)
		SELECT id, next_run_at
		  FROM scheduled_tasks
		 WHERE enabled = TRUE
		   AND (next_run_at IS NULL OR next_run_at <= NOW())
		 ORDER BY next_run_at ASC NULLS FIRST, id ASC
		 LIMIT 101
	`)
	if err != nil {
		t.Fatalf("explain schedule evaluation query: %v", err)
	}
	defer rows.Close()
	plan := make([]string, 0, 8)
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatalf("scan schedule evaluation plan: %v", err)
		}
		plan = append(plan, line)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read schedule evaluation plan: %v", err)
	}
	planText := strings.Join(plan, "\n")
	if !strings.Contains(planText, "idx_scheduled_tasks_enabled_next_run") {
		t.Fatalf("schedule evaluation did not use its partial index:\n%s", planText)
	}
	if strings.Contains(planText, "Sort") {
		t.Fatalf("schedule evaluation required an avoidable sort:\n%s", planText)
	}
}

func TestPostgresLiveActivityQuotaAndDeliveryClaimAreAtomic(t *testing.T) {
	store := newTestPostgresStore(t)
	ctx := context.Background()
	suffix := time.Now().UTC().UnixNano()
	username := fmt.Sprintf("live-activity-%d", suffix)
	user, err := store.CreateUserWithRole(username, "integration-test-hash", auth.RoleViewer, "local", "")
	if err != nil {
		t.Fatalf("create live-activity user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = store.pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, user.ID)
	})

	incidentID := fmt.Sprintf("incident-live-activity-%d", suffix)
	expiresAt := time.Now().UTC().Add(time.Hour)
	newToken := func(index int) LiveActivityPushToken {
		return LiveActivityPushToken{
			ID:              fmt.Sprintf("live-token-%d-%d", suffix, index),
			UserID:          user.ID,
			DeviceID:        fmt.Sprintf("device-%d", index),
			ActivityID:      fmt.Sprintf("activity-%d", index),
			IncidentID:      incidentID,
			TokenCiphertext: fmt.Sprintf("ciphertext-%d", index),
			TokenHash:       fmt.Sprintf("hash-%d-%d", suffix, index),
			BundleID:        "com.labtether.qa",
			Environment:     "sandbox",
			ExpiresAt:       expiresAt,
		}
	}
	for i := 0; i < MaxLiveActivityRegistrationsPerUser-1; i++ {
		if err := store.UpsertLiveActivityPushToken(ctx, newToken(i)); err != nil {
			t.Fatalf("seed live-activity token %d: %v", i, err)
		}
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i := MaxLiveActivityRegistrationsPerUser - 1; i <= MaxLiveActivityRegistrationsPerUser; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			results <- store.UpsertLiveActivityPushToken(ctx, newToken(index))
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)

	var succeeded, rejected int
	for err := range results {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrLiveActivityRegistrationLimit):
			rejected++
		default:
			t.Fatalf("unexpected live-activity registration result: %v", err)
		}
	}
	if succeeded != 1 || rejected != 1 {
		t.Fatalf("live-activity registrations succeeded=%d rejected=%d, want 1/1", succeeded, rejected)
	}

	var count int
	if err := store.pool.QueryRow(ctx, `SELECT COUNT(*) FROM live_activity_push_tokens WHERE user_id = $1`, user.ID).Scan(&count); err != nil {
		t.Fatalf("count live-activity registrations: %v", err)
	}
	if count != MaxLiveActivityRegistrationsPerUser {
		t.Fatalf("live-activity registration count=%d, want %d", count, MaxLiveActivityRegistrationsPerUser)
	}

	claimID := newToken(0).ID
	leaseUntil := time.Now().UTC().Add(30 * time.Second)
	claimResults := make(chan struct {
		generation int64
		claimed    bool
		err        error
	}, 2)
	start = make(chan struct{})
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			generation, claimed, err := store.ClaimLiveActivityPushDelivery(
				ctx, claimID, 0, "pending-state", time.Now().UTC(), leaseUntil, 1,
			)
			claimResults <- struct {
				generation int64
				claimed    bool
				err        error
			}{generation: generation, claimed: claimed, err: err}
		}()
	}
	close(start)
	wg.Wait()
	close(claimResults)

	var claimWinners int
	for result := range claimResults {
		if result.err != nil {
			t.Fatalf("claim live-activity delivery: %v", result.err)
		}
		if result.claimed {
			claimWinners++
			if result.generation != 1 {
				t.Fatalf("winning delivery generation=%d, want 1", result.generation)
			}
		}
	}
	if claimWinners != 1 {
		t.Fatalf("live-activity claim winners=%d, want 1", claimWinners)
	}

	if err := store.ClearLiveActivityPushRetry(ctx, claimID, 0, time.Now().UTC()); err != nil {
		t.Fatalf("apply stale live-activity completion: %v", err)
	}
	var generation int64
	var pending string
	var retryCount int
	if err := store.pool.QueryRow(ctx, `
		SELECT delivery_generation, pending_state_ciphertext, retry_count
		  FROM live_activity_push_tokens
		 WHERE id = $1
	`, claimID).Scan(&generation, &pending, &retryCount); err != nil {
		t.Fatalf("load claimed live-activity registration: %v", err)
	}
	if generation != 1 || pending != "pending-state" || retryCount != 1 {
		t.Fatalf("stale completion overwrote generation: generation=%d pending=%q retry=%d", generation, pending, retryCount)
	}
}

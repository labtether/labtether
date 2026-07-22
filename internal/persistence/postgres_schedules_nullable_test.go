package persistence

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/schedules"
)

// A schedule has no execution job until an occurrence is claimed. Both the
// original scheduled_tasks rows and newly created disabled definitions
// therefore legitimately store NULL in last_run_job_id.
func TestPostgresScheduledTaskNullLastRunJobIDSupportsEvaluationAndDelete(t *testing.T) {
	store := newTestPostgresStore(t)
	ctx := context.Background()
	suffix := time.Now().UTC().UnixNano()
	legacyID := fmt.Sprintf("schedule-null-job-legacy-%d", suffix)
	newID := fmt.Sprintf("schedule-null-job-new-%d", suffix)
	principalID := fmt.Sprintf("schedule-null-job-owner-%d", suffix)
	createdAt := time.Now().UTC().Truncate(time.Microsecond)

	t.Cleanup(func() {
		_, _ = store.pool.Exec(context.Background(),
			`DELETE FROM scheduled_tasks WHERE id = ANY($1::text[])`,
			[]string{legacyID, newID},
		)
	})

	// Represents a definition created before automatic execution initialized
	// next_run_at. Migration v90 adds last_run_job_id as nullable, so this row
	// must remain readable by the evaluator.
	if _, err := store.pool.Exec(ctx, `
		INSERT INTO scheduled_tasks
			(id, name, cron_expr, command, targets, enabled, created_by, created_at,
			 last_run_at, next_run_at, last_run_status, last_error, last_run_job_id)
		VALUES ($1, 'legacy nullable job', '0 * * * *', 'true', '["asset-qa"]'::jsonb,
			TRUE, $2, $3, NULL, NULL, '', '', NULL)
	`, legacyID, principalID, createdAt); err != nil {
		t.Fatalf("insert legacy nullable-job schedule: %v", err)
	}

	due, err := store.ListScheduledTasksForEvaluation(ctx, createdAt, schedules.MaxScheduledTaskPageSize)
	if err != nil {
		t.Fatalf("list nullable-job schedules for evaluation: %v", err)
	}
	foundLegacy := false
	for _, task := range due {
		if task.ID == legacyID {
			foundLegacy = true
			if task.LastRunJobID != "" || task.NextRunAt != nil {
				t.Fatalf("legacy schedule execution state = job %q next %v, want empty job and nil next run", task.LastRunJobID, task.NextRunAt)
			}
		}
	}
	if !foundLegacy {
		t.Fatalf("legacy nullable-job schedule %q was not returned for evaluation", legacyID)
	}

	// Represents the installed-product reproduction: a disabled definition is
	// created successfully, then DELETE first loads it for authorization.
	if err := store.CreateScheduledTask(ctx, schedules.ScheduledTask{
		ID:        newID,
		Name:      "new nullable job",
		CronExpr:  "0 * * * *",
		Command:   "true",
		Targets:   []string{"asset-qa"},
		Enabled:   false,
		CreatedBy: principalID,
		CreatedAt: createdAt.Add(time.Microsecond),
	}); err != nil {
		t.Fatalf("create nullable-job schedule: %v", err)
	}

	task, ok, err := store.GetScheduledTask(ctx, newID)
	if err != nil || !ok {
		t.Fatalf("load nullable-job schedule before delete: ok=%t err=%v", ok, err)
	}
	if task.LastRunJobID != "" {
		t.Fatalf("new schedule last_run_job_id = %q, want empty", task.LastRunJobID)
	}
	if err := store.DeleteScheduledTask(ctx, newID); err != nil {
		t.Fatalf("delete nullable-job schedule: %v", err)
	}
	if _, ok, err := store.GetScheduledTask(ctx, newID); err != nil || ok {
		t.Fatalf("load deleted nullable-job schedule: ok=%t err=%v", ok, err)
	}
}

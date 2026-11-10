package persistence

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/schedules"
)

// Pausing a schedule must not rewrite a run that already finished as
// "cancelled" (Schedules showed "Cancelled · 20:02" for a run that
// succeeded). Only a queued or running occurrence is cancelled.
func TestPostgresPausingScheduleKeepsFinishedRunResult(t *testing.T) {
	store := newTestPostgresStore(t)
	ctx := context.Background()
	suffix := time.Now().UTC().UnixNano()
	finishedID := fmt.Sprintf("schedule-pause-finished-%d", suffix)
	queuedID := fmt.Sprintf("schedule-pause-queued-%d", suffix)
	t.Cleanup(func() {
		_, _ = store.pool.Exec(context.Background(),
			`DELETE FROM scheduled_tasks WHERE id = ANY($1::text[])`, []string{finishedID, queuedID})
	})

	createdAt := time.Now().UTC().Truncate(time.Microsecond)
	for _, id := range []string{finishedID, queuedID} {
		if err := store.CreateScheduledTask(ctx, schedules.ScheduledTask{
			ID: id, Name: id, CronExpr: "0 * * * *", Command: "true", Targets: []string{"asset-qa"},
			Enabled: true, CreatedBy: "schedule-pause-owner", CreatedAt: createdAt,
		}); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	setRun := func(id, status, jobID, lastError string) {
		t.Helper()
		if _, err := store.pool.Exec(ctx,
			`UPDATE scheduled_tasks SET last_run_status = $2, last_run_job_id = $3, last_error = $4, last_run_at = $5 WHERE id = $1`,
			id, status, jobID, lastError, createdAt); err != nil {
			t.Fatalf("set run state for %s: %v", id, err)
		}
	}
	setRun(finishedID, "partial", "job-finished", "1 of 2 target commands failed or were blocked")
	setRun(queuedID, "queued", "job-queued", "")

	paused := false
	for _, id := range []string{finishedID, queuedID} {
		if err := store.UpdateScheduledTask(ctx, id, nil, nil, nil, nil, nil, &paused, schedules.NextRunUpdate{Set: true}, ""); err != nil {
			t.Fatalf("pause %s: %v", id, err)
		}
	}

	finished, ok, err := store.GetScheduledTask(ctx, finishedID)
	if err != nil || !ok {
		t.Fatalf("load finished schedule: ok=%t err=%v", ok, err)
	}
	if finished.Enabled || finished.LastRunStatus != "partial" || finished.LastRunJobID != "job-finished" ||
		finished.LastError != "1 of 2 target commands failed or were blocked" {
		t.Fatalf("paused finished schedule = enabled %t status %q job %q error %q, want paused with its partial run kept",
			finished.Enabled, finished.LastRunStatus, finished.LastRunJobID, finished.LastError)
	}

	queued, ok, err := store.GetScheduledTask(ctx, queuedID)
	if err != nil || !ok {
		t.Fatalf("load queued schedule: ok=%t err=%v", ok, err)
	}
	if queued.Enabled || queued.LastRunStatus != "cancelled" || queued.LastRunJobID != "" {
		t.Fatalf("paused queued schedule = enabled %t status %q job %q, want cancelled with no job",
			queued.Enabled, queued.LastRunStatus, queued.LastRunJobID)
	}
}

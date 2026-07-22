package schedulespkg

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/schedules"
)

const (
	defaultSchedulePollInterval = 15 * time.Second
	minSchedulePollInterval     = time.Second
	maxSchedulePollInterval     = 5 * time.Minute
	maxDueSchedulesPerPass      = 100
)

// EvaluationStats is returned by EvaluateDueSchedules for focused runtime
// verification and operational logging.
type EvaluationStats struct {
	Initialized int
	Invalidated int
	Claimed     int
	Backlog     bool
}

// RunScheduler evaluates due definitions immediately and then on a bounded
// interval until the hub runtime context is canceled.
func (d *Deps) RunScheduler(ctx context.Context) {
	if d == nil || d.ScheduleStore == nil || d.ExecutionStore == nil {
		<-ctx.Done()
		return
	}
	interval := shared.EnvOrDefaultDuration("SCHEDULE_POLL_INTERVAL", defaultSchedulePollInterval)
	if interval < minSchedulePollInterval {
		interval = minSchedulePollInterval
	}
	if interval > maxSchedulePollInterval {
		interval = maxSchedulePollInterval
	}

	evaluate := func() {
		stats, err := d.EvaluateDueSchedules(ctx, time.Now().UTC())
		if err != nil && ctx.Err() == nil {
			log.Printf("schedule runner: evaluation failed: %v", err)
		}
		if stats.Claimed > 0 || stats.Invalidated > 0 || stats.Backlog {
			log.Printf("schedule runner: claimed=%d initialized=%d invalidated=%d backlog=%t", stats.Claimed, stats.Initialized, stats.Invalidated, stats.Backlog)
		}
	}
	evaluate()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			evaluate()
		}
	}
}

// EvaluateDueSchedules performs one bounded scheduler pass. Claiming delegates
// to the persistence layer's atomic schedule-update + job-queue transaction.
func (d *Deps) EvaluateDueSchedules(ctx context.Context, now time.Time) (EvaluationStats, error) {
	var stats EvaluationStats
	if d == nil || d.ScheduleStore == nil || d.ExecutionStore == nil {
		return stats, errors.New("schedule execution dependencies are unavailable")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	tasks, err := d.ExecutionStore.ListScheduledTasksForEvaluation(ctx, now, maxDueSchedulesPerPass+1)
	if err != nil {
		return stats, err
	}
	if len(tasks) > maxDueSchedulesPerPass {
		stats.Backlog = true
		tasks = tasks[:maxDueSchedulesPerPass]
	}

	// Old configuration-only rows have no next_run_at. Initialize them to the
	// next future occurrence so enabling execution cannot produce a catch-up
	// storm on upgrade.
	var errs []error
	due := make([]schedules.ScheduledTask, 0, len(tasks))
	for _, task := range tasks {
		if ctx.Err() != nil {
			return stats, ctx.Err()
		}
		if !task.Enabled {
			continue
		}
		if _, parseErr := schedules.NextRun(task.CronExpr, now); parseErr != nil {
			if markErr := d.ExecutionStore.MarkScheduledTaskInvalid(ctx, task.ID, parseErr.Error()); markErr != nil {
				errs = append(errs, markErr)
			} else {
				stats.Invalidated++
			}
			continue
		}
		if task.NextRunAt == nil {
			next, nextErr := schedules.NextRun(task.CronExpr, now)
			if nextErr != nil {
				errs = append(errs, nextErr)
				continue
			}
			initialized, initErr := d.ExecutionStore.InitializeScheduledTaskNextRun(ctx, task.ID, next)
			if initErr != nil {
				errs = append(errs, initErr)
			} else if initialized {
				stats.Initialized++
			}
			continue
		}
		if !task.NextRunAt.After(now) {
			due = append(due, task)
		}
	}

	maxAttempts := 1
	if d.JobQueue != nil {
		maxAttempts = d.JobQueue.MaxAttempts()
	}
	for _, task := range due {
		if ctx.Err() != nil {
			return stats, ctx.Err()
		}
		dueAt := task.NextRunAt.UTC()
		next, nextErr := schedules.NextRun(task.CronExpr, now)
		if nextErr != nil {
			errs = append(errs, nextErr)
			continue
		}
		jobID := schedules.ExecutionJobID(task.ID, dueAt)
		job := schedules.ExecutionJob{
			JobID:        jobID,
			ScheduleID:   strings.TrimSpace(task.ID),
			ScheduleName: strings.TrimSpace(task.Name),
			ScheduledFor: dueAt,
			Command:      strings.TrimSpace(task.Command),
			Targets:      append([]string(nil), task.Targets...),
			GroupID:      strings.TrimSpace(task.GroupID),
			ActorID:      strings.TrimSpace(task.CreatedBy),
		}
		claimed, claimErr := d.ExecutionStore.ClaimScheduledTaskExecution(ctx, schedules.ExecutionClaim{
			ScheduleID:  task.ID,
			DueAt:       dueAt,
			ClaimedAt:   now,
			NextRunAt:   next,
			MaxAttempts: maxAttempts,
			Job:         job,
		})
		if claimErr != nil {
			errs = append(errs, claimErr)
			continue
		}
		if claimed {
			stats.Claimed++
		}
	}
	return stats, errors.Join(errs...)
}

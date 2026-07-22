package schedulespkg

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/schedules"
)

type runtimeScheduleStore struct {
	mu        sync.Mutex
	tasks     map[string]schedules.ScheduledTask
	claims    []schedules.ExecutionClaim
	claimErr  error
	invalidID []string
}

func newRuntimeScheduleStore(tasks ...schedules.ScheduledTask) *runtimeScheduleStore {
	store := &runtimeScheduleStore{tasks: make(map[string]schedules.ScheduledTask, len(tasks))}
	for _, task := range tasks {
		store.tasks[task.ID] = task
	}
	return store
}

func (s *runtimeScheduleStore) CreateScheduledTask(_ context.Context, task schedules.ScheduledTask) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks[task.ID] = task
	return nil
}

func (s *runtimeScheduleStore) GetScheduledTask(_ context.Context, id string) (schedules.ScheduledTask, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[id]
	return task, ok, nil
}

func (s *runtimeScheduleStore) ListScheduledTasks(_ context.Context, limit, offset int) ([]schedules.ScheduledTask, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]schedules.ScheduledTask, 0, len(s.tasks))
	for _, task := range s.tasks {
		copyTask := task
		copyTask.Targets = append([]string(nil), task.Targets...)
		if task.NextRunAt != nil {
			next := *task.NextRunAt
			copyTask.NextRunAt = &next
		}
		out = append(out, copyTask)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	total := len(out)
	if offset >= total {
		return []schedules.ScheduledTask{}, total, nil
	}
	if limit <= 0 || limit > total-offset {
		limit = total - offset
	}
	return out[offset : offset+limit], total, nil
}

func (s *runtimeScheduleStore) ListScheduledTasksForEvaluation(_ context.Context, now time.Time, limit int) ([]schedules.ScheduledTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]schedules.ScheduledTask, 0, len(s.tasks))
	for _, task := range s.tasks {
		if !task.Enabled || (task.NextRunAt != nil && task.NextRunAt.After(now)) {
			continue
		}
		copyTask := task
		copyTask.Targets = append([]string(nil), task.Targets...)
		if task.NextRunAt != nil {
			next := *task.NextRunAt
			copyTask.NextRunAt = &next
		}
		out = append(out, copyTask)
	}
	sort.Slice(out, func(i, j int) bool {
		left, right := out[i].NextRunAt, out[j].NextRunAt
		if left == nil || right == nil {
			if left == nil && right == nil {
				return out[i].ID < out[j].ID
			}
			return left == nil
		}
		if left.Equal(*right) {
			return out[i].ID < out[j].ID
		}
		return left.Before(*right)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *runtimeScheduleStore) UpdateScheduledTask(_ context.Context, _ string, _ *string, _ *string, _ *string, _ *[]string, _ *string, _ *bool, _ schedules.NextRunUpdate) error {
	return nil
}

func (s *runtimeScheduleStore) DeleteScheduledTask(_ context.Context, _ string) error { return nil }

func (s *runtimeScheduleStore) InitializeScheduledTaskNextRun(_ context.Context, id string, next time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[id]
	if !ok || !task.Enabled || task.NextRunAt != nil {
		return false, nil
	}
	next = next.UTC()
	task.NextRunAt = &next
	s.tasks[id] = task
	return true, nil
}

func (s *runtimeScheduleStore) ClaimScheduledTaskExecution(_ context.Context, claim schedules.ExecutionClaim) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.claimErr != nil {
		return false, s.claimErr
	}
	task, ok := s.tasks[claim.ScheduleID]
	if !ok || !task.Enabled || task.NextRunAt == nil || !task.NextRunAt.Equal(claim.DueAt) || task.LastRunStatus == "queued" || task.LastRunStatus == "running" {
		return false, nil
	}
	next := claim.NextRunAt.UTC()
	task.NextRunAt = &next
	task.LastRunStatus = "queued"
	task.LastRunJobID = claim.Job.JobID
	s.tasks[claim.ScheduleID] = task
	s.claims = append(s.claims, claim)
	return true, nil
}

func TestEvaluateDueSchedulesDoesNotOverlapRunningOccurrence(t *testing.T) {
	now := time.Now().UTC()
	due := now.Add(-time.Minute)
	store := newRuntimeScheduleStore(schedules.ScheduledTask{
		ID: "running", Enabled: true, CronExpr: "* * * * *", NextRunAt: &due,
		LastRunStatus: "running", LastRunJobID: "existing-job",
	})
	deps := &Deps{ScheduleStore: store, ExecutionStore: store}
	stats, err := deps.EvaluateDueSchedules(context.Background(), now)
	if err != nil {
		t.Fatalf("EvaluateDueSchedules() error = %v", err)
	}
	if stats.Claimed != 0 || len(store.claims) != 0 {
		t.Fatalf("stats/claims = %+v/%d, want no overlap", stats, len(store.claims))
	}
}

func (s *runtimeScheduleStore) MarkScheduledTaskInvalid(_ context.Context, id, message string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	task := s.tasks[id]
	task.Enabled = false
	task.NextRunAt = nil
	task.LastRunStatus = "invalid"
	task.LastError = message
	s.tasks[id] = task
	s.invalidID = append(s.invalidID, id)
	return nil
}

func (s *runtimeScheduleStore) BeginScheduledTaskExecution(_ context.Context, scheduleID, jobID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[scheduleID]
	if !ok || !task.Enabled || task.LastRunJobID != jobID || task.LastRunStatus != "queued" {
		return false, nil
	}
	task.LastRunStatus = "running"
	s.tasks[scheduleID] = task
	return true, nil
}

func (s *runtimeScheduleStore) CompleteScheduledTaskExecution(_ context.Context, _, _, _, _ string, _ time.Time) error {
	return nil
}

func TestEvaluateDueSchedulesSkipsDisabledAndFutureAndInitializesLegacy(t *testing.T) {
	now := time.Date(2026, time.July, 14, 3, 12, 0, 0, time.UTC)
	past := now.Add(-time.Minute)
	future := now.Add(time.Minute)
	store := newRuntimeScheduleStore(
		schedules.ScheduledTask{ID: "disabled", Enabled: false, CronExpr: "* * * * *", NextRunAt: &past},
		schedules.ScheduledTask{ID: "future", Enabled: true, CronExpr: "* * * * *", NextRunAt: &future},
		schedules.ScheduledTask{ID: "legacy", Enabled: true, CronExpr: "@hourly"},
	)
	deps := &Deps{ScheduleStore: store, ExecutionStore: store}

	stats, err := deps.EvaluateDueSchedules(context.Background(), now)
	if err != nil {
		t.Fatalf("EvaluateDueSchedules() error = %v", err)
	}
	if stats.Initialized != 1 || stats.Claimed != 0 || stats.Invalidated != 0 {
		t.Fatalf("stats = %+v", stats)
	}
	if len(store.claims) != 0 {
		t.Fatalf("claims = %d, want 0", len(store.claims))
	}
	legacy := store.tasks["legacy"]
	if legacy.NextRunAt == nil || !legacy.NextRunAt.After(now) {
		t.Fatalf("legacy next run = %v, want future time", legacy.NextRunAt)
	}
	second, err := deps.EvaluateDueSchedules(context.Background(), now)
	if err != nil {
		t.Fatalf("second EvaluateDueSchedules() error = %v", err)
	}
	if second.Initialized != 0 || second.Claimed != 0 || len(store.claims) != 0 {
		t.Fatalf("legacy definition replayed on second pass: stats=%+v claims=%d", second, len(store.claims))
	}
}

func TestEvaluateDueSchedulesConcurrentWorkersClaimOccurrenceOnce(t *testing.T) {
	now := time.Date(2026, time.July, 14, 3, 12, 0, 0, time.UTC)
	due := now.Add(-time.Minute)
	store := newRuntimeScheduleStore(schedules.ScheduledTask{
		ID: "due", Name: "Due", Enabled: true, CronExpr: "* * * * *", Command: "uptime",
		Targets: []string{"asset-1"}, CreatedBy: "owner", NextRunAt: &due,
	})
	deps := &Deps{ScheduleStore: store, ExecutionStore: store}

	var wg sync.WaitGroup
	stats := make([]EvaluationStats, 2)
	errs := make([]error, 2)
	for index := range stats {
		wg.Add(1)
		go func() {
			defer wg.Done()
			stats[index], errs[index] = deps.EvaluateDueSchedules(context.Background(), now)
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatalf("EvaluateDueSchedules() error = %v", err)
		}
	}
	if stats[0].Claimed+stats[1].Claimed != 1 {
		t.Fatalf("claimed counts = %d + %d, want 1", stats[0].Claimed, stats[1].Claimed)
	}
	if len(store.claims) != 1 {
		t.Fatalf("durable claims = %d, want 1", len(store.claims))
	}
	claim := store.claims[0]
	if claim.Job.JobID != schedules.ExecutionJobID("due", due) {
		t.Fatalf("job id = %q, want deterministic occurrence ID", claim.Job.JobID)
	}
	if claim.MaxAttempts != 1 {
		t.Fatalf("max attempts = %d, want bounded fallback 1", claim.MaxAttempts)
	}
}

func TestEvaluateDueSchedulesDrainsBoundedBacklogAcrossPasses(t *testing.T) {
	now := time.Date(2026, time.July, 14, 3, 12, 0, 0, time.UTC)
	due := now.Add(-time.Minute)
	tasks := make([]schedules.ScheduledTask, 0, 250)
	for i := 0; i < 250; i++ {
		tasks = append(tasks, schedules.ScheduledTask{
			ID: fmt.Sprintf("due-%03d", i), Name: "Due", Enabled: true, CronExpr: "* * * * *", Command: "uptime",
			Targets: []string{"asset-1"}, CreatedBy: "owner", NextRunAt: &due,
		})
	}
	store := newRuntimeScheduleStore(tasks...)
	deps := &Deps{ScheduleStore: store, ExecutionStore: store}

	wantClaimed := []int{100, 100, 50}
	wantBacklog := []bool{true, true, false}
	for pass := range wantClaimed {
		stats, err := deps.EvaluateDueSchedules(context.Background(), now)
		if err != nil {
			t.Fatalf("pass %d error = %v", pass+1, err)
		}
		if stats.Claimed != wantClaimed[pass] || stats.Backlog != wantBacklog[pass] {
			t.Fatalf("pass %d stats=%+v want claimed=%d backlog=%t", pass+1, stats, wantClaimed[pass], wantBacklog[pass])
		}
	}
	if len(store.claims) != len(tasks) {
		t.Fatalf("claimed=%d want %d after bounded drain", len(store.claims), len(tasks))
	}
}

func TestEvaluateDueSchedulesInvalidCronIsDisabled(t *testing.T) {
	store := newRuntimeScheduleStore(schedules.ScheduledTask{ID: "invalid", Enabled: true, CronExpr: "bad"})
	deps := &Deps{ScheduleStore: store, ExecutionStore: store}
	stats, err := deps.EvaluateDueSchedules(context.Background(), time.Now().UTC())
	if err != nil {
		t.Fatalf("EvaluateDueSchedules() error = %v", err)
	}
	if stats.Invalidated != 1 || store.tasks["invalid"].Enabled {
		t.Fatalf("stats/task = %+v/%+v", stats, store.tasks["invalid"])
	}
}

func TestEvaluateDueSchedulesClaimFailureIsReported(t *testing.T) {
	now := time.Now().UTC()
	due := now.Add(-time.Minute)
	store := newRuntimeScheduleStore(schedules.ScheduledTask{ID: "due", Enabled: true, CronExpr: "@hourly", NextRunAt: &due})
	store.claimErr = errors.New("database unavailable")
	deps := &Deps{ScheduleStore: store, ExecutionStore: store}
	stats, err := deps.EvaluateDueSchedules(context.Background(), now)
	if !errors.Is(err, store.claimErr) {
		t.Fatalf("error = %v, want %v", err, store.claimErr)
	}
	if stats.Claimed != 0 {
		t.Fatalf("stats = %+v", stats)
	}
}

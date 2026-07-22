package schedulespkg

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/schedules"
)

// interleavingScheduleStore deterministically models the scheduler claiming a
// due occurrence after the PATCH handler reads the definition but before its
// update reaches persistence.
type interleavingScheduleStore struct {
	task          schedules.ScheduledTask
	claimedNext   time.Time
	nextRunWasSet bool
}

func (s *interleavingScheduleStore) CreateScheduledTask(context.Context, schedules.ScheduledTask) error {
	return nil
}

func (s *interleavingScheduleStore) GetScheduledTask(_ context.Context, id string) (schedules.ScheduledTask, bool, error) {
	if id != s.task.ID {
		return schedules.ScheduledTask{}, false, nil
	}
	copyTask := s.task
	if s.task.NextRunAt != nil {
		next := *s.task.NextRunAt
		copyTask.NextRunAt = &next
	}
	return copyTask, true, nil
}

func (s *interleavingScheduleStore) ListScheduledTasks(context.Context, int, int) ([]schedules.ScheduledTask, int, error) {
	return []schedules.ScheduledTask{s.task}, 1, nil
}

func (s *interleavingScheduleStore) UpdateScheduledTask(
	_ context.Context,
	id string,
	name, cronExpr, command *string,
	targets *[]string,
	groupID *string,
	enabled *bool,
	nextRun schedules.NextRunUpdate,
) error {
	// The production claim transaction wins this interleaving first.
	claimedNext := s.claimedNext.UTC()
	s.task.NextRunAt = &claimedNext
	s.task.LastRunStatus = "queued"
	s.task.LastRunJobID = "claimed-job"

	if id != s.task.ID {
		return nil
	}
	if name != nil {
		s.task.Name = *name
	}
	if cronExpr != nil {
		s.task.CronExpr = *cronExpr
	}
	if command != nil {
		s.task.Command = *command
	}
	if targets != nil {
		s.task.Targets = append([]string(nil), (*targets)...)
	}
	if groupID != nil {
		s.task.GroupID = *groupID
	}
	if enabled != nil {
		s.task.Enabled = *enabled
	}
	s.nextRunWasSet = nextRun.Set
	if nextRun.Set {
		s.task.NextRunAt = nextRun.Value
	}
	return nil
}

func (s *interleavingScheduleStore) DeleteScheduledTask(context.Context, string) error { return nil }

func TestScheduleDefinitionEditCannotRollBackConcurrentOccurrenceClaim(t *testing.T) {
	now := time.Date(2026, time.July, 14, 6, 0, 0, 0, time.UTC)
	due := now.Add(-time.Minute)
	claimedNext := now.Add(time.Hour)

	for _, testCase := range []struct {
		name string
		body string
	}{
		{name: "rename", body: `{"name":"renamed"}`},
		{name: "command edit", body: `{"command":"whoami"}`},
		{name: "idempotent enable", body: `{"enabled":true}`},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			store := &interleavingScheduleStore{
				task: schedules.ScheduledTask{
					ID: "schedule-1", Name: "before", CronExpr: "@hourly", Command: "uptime",
					Targets: []string{"asset-1"}, Enabled: true, CreatedBy: "owner", NextRunAt: &due,
				},
				claimedNext: claimedNext,
			}
			deps := &Deps{ScheduleStore: store}
			request := httptest.NewRequest(http.MethodPatch, "/api/v2/schedules/schedule-1", strings.NewReader(testCase.body))
			request = request.WithContext(apiv2.ContextWithPrincipal(request.Context(), "owner", "owner"))
			recorder := httptest.NewRecorder()

			deps.V2UpdateSchedule(recorder, request, "schedule-1")

			if recorder.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			if store.nextRunWasSet {
				t.Fatal("definition-only edit attempted to write a stale next_run_at snapshot")
			}
			if store.task.NextRunAt == nil || !store.task.NextRunAt.Equal(claimedNext) {
				t.Fatalf("next_run_at=%v want concurrent claim value %s", store.task.NextRunAt, claimedNext)
			}
			if store.task.LastRunStatus != "queued" || store.task.LastRunJobID != "claimed-job" {
				t.Fatalf("claim state was lost: %+v", store.task)
			}
		})
	}
}

package schedulespkg

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/schedules"
)

func TestGetScheduleReturnsLastRunJobLinkage(t *testing.T) {
	store := persistence.NewMemoryScheduleStore()
	task := schedules.ScheduledTask{
		ID:            "schedule-1",
		Name:          "linked run",
		CronExpr:      "@hourly",
		Command:       "uname -s",
		Targets:       []string{"asset-1"},
		Enabled:       true,
		CreatedBy:     "owner",
		CreatedAt:     time.Now().UTC(),
		LastRunStatus: "succeeded",
		LastRunJobID:  "schedrun_expected",
	}
	if err := store.CreateScheduledTask(context.Background(), task); err != nil {
		t.Fatalf("seed schedule: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v2/schedules/"+task.ID, nil)
	(&Deps{ScheduleStore: store}).V2GetSchedule(recorder, request, task.ID)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	var response struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	encoded, ok := response.Data["last_run_job_id"]
	if !ok {
		t.Fatalf("last_run_job_id missing from response: %s", recorder.Body.String())
	}
	var jobID string
	if err := json.Unmarshal(encoded, &jobID); err != nil {
		t.Fatalf("decode last_run_job_id: %v", err)
	}
	if jobID != task.LastRunJobID {
		t.Fatalf("last_run_job_id=%q want %q", jobID, task.LastRunJobID)
	}
}

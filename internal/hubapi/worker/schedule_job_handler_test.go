package worker

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/groups"
	"github.com/labtether/labtether/internal/hubapi/groupfeatures"
	"github.com/labtether/labtether/internal/jobqueue"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/policy"
	"github.com/labtether/labtether/internal/schedules"
	"github.com/labtether/labtether/internal/terminal"
)

type recordingScheduleExecutionStore struct {
	mu      sync.Mutex
	status  string
	error   string
	jobID   string
	updates int
	failErr error
}

type failingListAssetStore struct {
	persistence.AssetStore
}

func (failingListAssetStore) ListAssets() ([]assets.Asset, error) {
	return nil, errors.New("transient inventory failure")
}

func (*recordingScheduleExecutionStore) InitializeScheduledTaskNextRun(context.Context, string, time.Time) (bool, error) {
	return false, nil
}

func (*recordingScheduleExecutionStore) ListScheduledTasksForEvaluation(context.Context, time.Time, int) ([]schedules.ScheduledTask, error) {
	return nil, nil
}

func (*recordingScheduleExecutionStore) ClaimScheduledTaskExecution(context.Context, schedules.ExecutionClaim) (bool, error) {
	return false, nil
}

func (*recordingScheduleExecutionStore) BeginScheduledTaskExecution(context.Context, string, string) (bool, error) {
	return true, nil
}

func (*recordingScheduleExecutionStore) MarkScheduledTaskInvalid(context.Context, string, string) error {
	return nil
}

func (s *recordingScheduleExecutionStore) CompleteScheduledTaskExecution(_ context.Context, _, jobID, status, errorMessage string, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobID = jobID
	s.status = status
	s.error = errorMessage
	s.updates++
	return s.failErr
}

func setupScheduleWorkerTest(t *testing.T, target string) (*Deps, schedules.ExecutionJob, *recordingScheduleExecutionStore, *persistence.MemoryActionStore) {
	t.Helper()
	scheduleStore := persistence.NewMemoryScheduleStore()
	now := time.Now().UTC()
	execution := schedules.ExecutionJob{
		JobID:        schedules.ExecutionJobID("sched-1", now),
		ScheduleID:   "sched-1",
		ScheduleName: "Health check",
		ScheduledFor: now,
		Command:      "uptime",
		Targets:      []string{target},
		ActorID:      "owner",
	}
	if err := scheduleStore.CreateScheduledTask(context.Background(), schedules.ScheduledTask{
		ID: "sched-1", Name: "Health check", CronExpr: "@hourly", Command: "uptime",
		Targets: []string{target}, Enabled: true, CreatedBy: "owner", CreatedAt: now,
		LastRunStatus: "queued", LastRunJobID: execution.JobID,
	}); err != nil {
		t.Fatalf("create schedule: %v", err)
	}
	assetStore := persistence.NewMemoryAssetStore()
	if _, err := assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{AssetID: target, Name: target, Status: "online"}); err != nil {
		t.Fatalf("create asset: %v", err)
	}
	actionStore := persistence.NewMemoryActionStore()
	completionStore := &recordingScheduleExecutionStore{}
	deps := &Deps{
		ScheduleStore:          scheduleStore,
		ScheduleExecutionStore: completionStore,
		ActionStore:            actionStore,
		AssetStore:             assetStore,
		GroupStore:             persistence.NewMemoryGroupStore(),
		AuditStore:             persistence.NewMemoryAuditStore(),
		AuthorizeScheduleTarget: func(context.Context, string, string) error {
			return nil
		},
		GetPolicyConfig: func() policy.EvaluatorConfig { return policy.DefaultEvaluatorConfig() },
		EvaluateAssetGuardrails: func(string, time.Time) (groupfeatures.GroupMaintenanceGuardrails, error) {
			return groupfeatures.GroupMaintenanceGuardrails{}, nil
		},
	}
	return deps, execution, completionStore, actionStore
}

func scheduleQueueJob(t *testing.T, execution schedules.ExecutionJob) *jobqueue.Job {
	t.Helper()
	payload, err := json.Marshal(execution)
	if err != nil {
		t.Fatalf("marshal schedule job: %v", err)
	}
	return &jobqueue.Job{ID: execution.JobID, Kind: jobqueue.KindScheduleRun, Payload: payload, Attempts: 1}
}

func TestScheduleWorkerExecutesThroughAgentWithBoundedTimeout(t *testing.T) {
	deps, execution, completion, actionStore := setupScheduleWorkerTest(t, "asset-1")
	deps.AgentMgr = agentmgr.NewManager()
	deps.AgentMgr.Register(agentmgr.NewAgentConn(nil, "asset-1", "linux"))
	capturedTimeout := 0
	deps.ExecuteViaAgent = func(job terminal.CommandJob) terminal.CommandResult {
		capturedTimeout = job.TimeoutSec
		return terminal.CommandResult{
			JobID: job.JobID, SessionID: job.SessionID, CommandID: job.CommandID,
			Status: "succeeded", Output: "ok", CompletedAt: time.Now().UTC(),
		}
	}

	var processed atomic.Uint64
	if err := deps.HandleScheduleRunJob(&processed)(context.Background(), scheduleQueueJob(t, execution)); err != nil {
		t.Fatalf("HandleScheduleRunJob() error = %v", err)
	}
	if capturedTimeout != scheduleTargetTimeoutSecond {
		t.Fatalf("agent timeout = %d, want %d", capturedTimeout, scheduleTargetTimeoutSecond)
	}
	if completion.status != "succeeded" || completion.updates != 1 || completion.jobID != execution.JobID {
		t.Fatalf("completion = %+v", completion)
	}
	runs, err := actionStore.ListActionRuns(10, 0, "", "")
	if err != nil {
		t.Fatalf("list action runs: %v", err)
	}
	if len(runs) != 1 || runs[0].Status != "succeeded" || runs[0].Params["schedule_id"] != execution.ScheduleID {
		t.Fatalf("action runs = %+v", runs)
	}
	if processed.Load() != 1 {
		t.Fatalf("processed = %d, want 1", processed.Load())
	}
}

func TestScheduleWorkerMaintenanceBlockPreventsCommandDispatch(t *testing.T) {
	deps, execution, completion, actionStore := setupScheduleWorkerTest(t, "asset-1")
	executions := 0
	deps.EvaluateAssetGuardrails = func(string, time.Time) (groupfeatures.GroupMaintenanceGuardrails, error) {
		return groupfeatures.GroupMaintenanceGuardrails{BlockActions: true}, nil
	}
	deps.ExecuteViaAgent = func(terminal.CommandJob) terminal.CommandResult {
		executions++
		return terminal.CommandResult{}
	}

	var processed atomic.Uint64
	if err := deps.HandleScheduleRunJob(&processed)(context.Background(), scheduleQueueJob(t, execution)); err != nil {
		t.Fatalf("HandleScheduleRunJob() error = %v", err)
	}
	if executions != 0 {
		t.Fatalf("executions = %d, want 0", executions)
	}
	if completion.status != "blocked" {
		t.Fatalf("completion status = %q, want blocked", completion.status)
	}
	runs, err := actionStore.ListActionRuns(10, 0, "", "")
	if err != nil {
		t.Fatalf("list action runs: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("action runs = %+v, want none", runs)
	}
}

func TestScheduleWorkerUnknownTargetFailsWithoutDispatchOrRetry(t *testing.T) {
	deps, execution, completion, _ := setupScheduleWorkerTest(t, "asset-1")
	execution.Targets = []string{"missing"}
	executions := 0
	deps.ExecuteViaAgent = func(terminal.CommandJob) terminal.CommandResult {
		executions++
		return terminal.CommandResult{}
	}

	var processed atomic.Uint64
	if err := deps.HandleScheduleRunJob(&processed)(context.Background(), scheduleQueueJob(t, execution)); err != nil {
		t.Fatalf("HandleScheduleRunJob() error = %v", err)
	}
	if executions != 0 || completion.status != "failed" || completion.error == "" {
		t.Fatalf("executions/completion = %d/%+v", executions, completion)
	}
}

func TestScheduleWorkerPreDispatchInfrastructureFailureUsesBoundedQueueRetry(t *testing.T) {
	deps, execution, completion, _ := setupScheduleWorkerTest(t, "asset-1")
	deps.AssetStore = failingListAssetStore{AssetStore: deps.AssetStore}
	var processed atomic.Uint64
	err := deps.HandleScheduleRunJob(&processed)(context.Background(), scheduleQueueJob(t, execution))
	if err == nil {
		t.Fatal("HandleScheduleRunJob() unexpectedly succeeded")
	}
	if completion.updates != 0 || processed.Load() != 0 {
		t.Fatalf("completion/processed = %d/%d, want 0/0 before queue retry", completion.updates, processed.Load())
	}
}

func TestScheduleWorkerFinalizedRedeliveryDoesNotReplayCommand(t *testing.T) {
	deps, execution, completion, _ := setupScheduleWorkerTest(t, "asset-1")
	if err := deps.ScheduleStore.DeleteScheduledTask(context.Background(), execution.ScheduleID); err != nil {
		t.Fatalf("delete queued schedule: %v", err)
	}
	if err := deps.ScheduleStore.CreateScheduledTask(context.Background(), schedules.ScheduledTask{
		ID: execution.ScheduleID, Enabled: true, CronExpr: "@hourly", Command: execution.Command,
		Targets: execution.Targets, CreatedBy: execution.ActorID,
		LastRunStatus: "succeeded", LastRunJobID: execution.JobID,
	}); err != nil {
		t.Fatalf("create finalized schedule: %v", err)
	}
	executions := 0
	deps.ExecuteViaAgent = func(terminal.CommandJob) terminal.CommandResult {
		executions++
		return terminal.CommandResult{}
	}
	var processed atomic.Uint64
	job := scheduleQueueJob(t, execution)
	job.Attempts = 2
	if err := deps.HandleScheduleRunJob(&processed)(context.Background(), job); err != nil {
		t.Fatalf("HandleScheduleRunJob() error = %v", err)
	}
	if executions != 0 || completion.updates != 0 || processed.Load() != 1 {
		t.Fatalf("executions/completions/processed = %d/%d/%d", executions, completion.updates, processed.Load())
	}
}

func TestScheduleWorkerInterruptedAttemptIsFailedWithoutReplay(t *testing.T) {
	deps, execution, completion, _ := setupScheduleWorkerTest(t, "asset-1")
	if err := deps.ScheduleStore.DeleteScheduledTask(context.Background(), execution.ScheduleID); err != nil {
		t.Fatalf("delete queued schedule: %v", err)
	}
	if err := deps.ScheduleStore.CreateScheduledTask(context.Background(), schedules.ScheduledTask{
		ID: execution.ScheduleID, Enabled: true, CronExpr: "@hourly", Command: execution.Command,
		Targets: execution.Targets, CreatedBy: execution.ActorID,
		LastRunStatus: "running", LastRunJobID: execution.JobID,
	}); err != nil {
		t.Fatalf("create running schedule: %v", err)
	}
	executions := 0
	deps.ExecuteViaAgent = func(terminal.CommandJob) terminal.CommandResult {
		executions++
		return terminal.CommandResult{}
	}
	var processed atomic.Uint64
	job := scheduleQueueJob(t, execution)
	job.Attempts = 2
	if err := deps.HandleScheduleRunJob(&processed)(context.Background(), job); err != nil {
		t.Fatalf("HandleScheduleRunJob() error = %v", err)
	}
	if executions != 0 || completion.status != "failed" || completion.updates != 1 {
		t.Fatalf("executions/completion = %d/%+v", executions, completion)
	}
}

func TestScheduleWorkerCompletionFailureRetriesFinalizationWithoutReplay(t *testing.T) {
	deps, execution, completion, _ := setupScheduleWorkerTest(t, "asset-1")
	deps.AgentMgr = agentmgr.NewManager()
	deps.AgentMgr.Register(agentmgr.NewAgentConn(nil, "asset-1", "linux"))
	executions := 0
	deps.ExecuteViaAgent = func(job terminal.CommandJob) terminal.CommandResult {
		executions++
		return terminal.CommandResult{
			JobID: job.JobID, SessionID: job.SessionID, CommandID: job.CommandID,
			Status: actions.StatusSucceeded, CompletedAt: time.Now().UTC(),
		}
	}
	completion.failErr = errors.New("transient completion persistence failure")

	var processed atomic.Uint64
	job := scheduleQueueJob(t, execution)
	err := deps.HandleScheduleRunJob(&processed)(context.Background(), job)
	if !errors.Is(err, completion.failErr) {
		t.Fatalf("first attempt error = %v, want completion persistence failure", err)
	}
	if executions != 1 || processed.Load() != 0 {
		t.Fatalf("executions/processed = %d/%d, want 1/0", executions, processed.Load())
	}
	// Production's BeginScheduledTaskExecution CAS leaves the definition in the
	// running state before commands dispatch. The focused test uses separate
	// schedule/completion stubs, so mirror that durable state before redelivery.
	if err := deps.ScheduleStore.DeleteScheduledTask(context.Background(), execution.ScheduleID); err != nil {
		t.Fatalf("replace queued schedule: %v", err)
	}
	if err := deps.ScheduleStore.CreateScheduledTask(context.Background(), schedules.ScheduledTask{
		ID: execution.ScheduleID, Enabled: true, CronExpr: "@hourly", Command: execution.Command,
		Targets: execution.Targets, CreatedBy: execution.ActorID,
		LastRunStatus: "running", LastRunJobID: execution.JobID,
	}); err != nil {
		t.Fatalf("create running schedule state: %v", err)
	}

	completion.failErr = nil
	job.Attempts = 2
	if err := deps.HandleScheduleRunJob(&processed)(context.Background(), job); err != nil {
		t.Fatalf("redelivery finalization error = %v", err)
	}
	if executions != 1 {
		t.Fatalf("redelivery replayed command: executions=%d", executions)
	}
	if completion.status != actions.StatusFailed || completion.updates != 2 || processed.Load() != 1 {
		t.Fatalf("completion/processed = %+v/%d", completion, processed.Load())
	}
}

func TestScheduleDeadLetterMarksOccurrenceFailed(t *testing.T) {
	deps, execution, completion, _ := setupScheduleWorkerTest(t, "asset-1")
	job := scheduleQueueJob(t, execution)
	job.Attempts = 3
	deps.RecordDeadLetter(context.Background(), job, errors.New("pre-dispatch infrastructure failure"))
	if completion.status != "failed" || completion.updates != 1 || completion.error == "" {
		t.Fatalf("completion = %+v", completion)
	}
}

func TestResolveScheduleExecutionTargetsIncludesDescendantGroupsAndDeduplicates(t *testing.T) {
	groupStore := persistence.NewMemoryGroupStore()
	parent, err := groupStore.CreateGroup(groups.CreateRequest{Name: "Parent", Slug: "parent"})
	if err != nil {
		t.Fatalf("create parent group: %v", err)
	}
	child, err := groupStore.CreateGroup(groups.CreateRequest{Name: "Child", Slug: "child", ParentGroupID: parent.ID})
	if err != nil {
		t.Fatalf("create child group: %v", err)
	}
	assetStore := persistence.NewMemoryAssetStore()
	for _, heartbeat := range []assets.HeartbeatRequest{
		{AssetID: "parent-asset", GroupID: parent.ID},
		{AssetID: "child-asset", GroupID: child.ID},
	} {
		if _, err := assetStore.UpsertAssetHeartbeat(heartbeat); err != nil {
			t.Fatalf("create asset: %v", err)
		}
	}
	deps := &Deps{AssetStore: assetStore, GroupStore: groupStore}
	targets, err := deps.resolveScheduleExecutionTargets(schedules.ExecutionJob{
		Targets: []string{"child-asset"},
		GroupID: parent.ID,
	})
	if err != nil {
		t.Fatalf("resolveScheduleExecutionTargets() error = %v", err)
	}
	if len(targets) != 2 || targets[0] != "child-asset" || targets[1] != "parent-asset" {
		t.Fatalf("targets = %#v", targets)
	}
}

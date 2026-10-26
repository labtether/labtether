package worker

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/groupmaintenance"
	"github.com/labtether/labtether/internal/groups"
	"github.com/labtether/labtether/internal/hubapi/groupfeatures"
	"github.com/labtether/labtether/internal/jobqueue"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/schedules"
	"github.com/labtether/labtether/internal/terminal"
)

// Persist dispatch/completion in the real Memory schedule store so a second
// worker delivery observes the first one's state. This serial fixture adapts
// the existing handler adapter, not PostgreSQL's atomic queue/claim protocol.
type persistedScheduleExecution struct {
	recordingScheduleExecutionStore
	store persistence.ScheduleStore
}

func (s *persistedScheduleExecution) replace(ctx context.Context, task schedules.ScheduledTask) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.store.DeleteScheduledTask(ctx, task.ID); err != nil {
		return err
	}
	return s.store.CreateScheduledTask(ctx, task)
}

func (s *persistedScheduleExecution) BeginScheduledTaskExecution(ctx context.Context, id, job string) (bool, error) {
	task, found, err := s.store.GetScheduledTask(ctx, id)
	if err != nil || !found || !task.Enabled || task.LastRunJobID != job || task.LastRunStatus != "queued" {
		return false, err
	}
	task.LastRunStatus = "running"
	return true, s.replace(ctx, task)
}

func (s *persistedScheduleExecution) CompleteScheduledTaskExecution(ctx context.Context, id, job, result, reason string, _ time.Time) error {
	task, found, err := s.store.GetScheduledTask(ctx, id)
	if err != nil || !found || task.LastRunJobID != job {
		return err
	}
	task.LastRunStatus, task.LastError = result, reason
	return s.replace(ctx, task)
}

type scheduleAssetReads struct {
	*persistence.MemoryAssetStore
	list func(context.Context) error
	get  func(context.Context, string) error
}

func (s scheduleAssetReads) ListAssets() ([]assets.Asset, error) {
	return nil, errors.New("legacy inventory must not be used")
}

func (s scheduleAssetReads) ListAssetsContext(ctx context.Context) ([]assets.Asset, error) {
	if s.list != nil {
		if err := s.list(ctx); err != nil {
			return nil, err
		}
	}
	return s.MemoryAssetStore.ListAssetsContext(ctx)
}

func (s scheduleAssetReads) GetAssetContext(ctx context.Context, id string) (assets.Asset, bool, error) {
	if s.get != nil {
		if err := s.get(ctx, id); err != nil {
			return assets.Asset{}, false, err
		}
	}
	return s.MemoryAssetStore.GetAssetContext(ctx, id)
}

type scheduleGroupReads struct {
	*persistence.MemoryGroupStore
	read func(context.Context) error
}

func (s scheduleGroupReads) ListGroups() ([]groups.Group, error) {
	return nil, errors.New("legacy inventory must not be used")
}

func (s scheduleGroupReads) ListGroupsContext(ctx context.Context) ([]groups.Group, error) {
	if err := s.read(ctx); err != nil {
		return nil, err
	}
	return s.MemoryGroupStore.ListGroupsContext(ctx)
}

func persistentScheduleFixture(t *testing.T) (*Deps, schedules.ExecutionJob) {
	t.Helper()
	d, execution, _, _ := setupScheduleWorkerTest(t, "asset-1")
	d.ScheduleExecutionStore = &persistedScheduleExecution{store: d.ScheduleStore}
	return d, execution
}

func requireScheduleState(t *testing.T, d *Deps, execution schedules.ExecutionJob, want string, children int) {
	t.Helper()
	task, found, err := d.ScheduleStore.GetScheduledTask(context.Background(), execution.ScheduleID)
	if err != nil || !found || task.LastRunStatus != want || task.LastRunJobID != execution.JobID {
		t.Fatalf("durable schedule = %+v, found=%v err=%v, want %s", task, found, err, want)
	}
	runs, err := d.ActionStore.ListActionRuns(10, 0, "", "")
	if err != nil || len(runs) != children {
		t.Fatalf("persisted child actions = %+v err=%v, want %d", runs, err, children)
	}
	if children == 1 && runs[0].Status != actions.StatusSucceeded {
		t.Fatalf("child outcome = %s", runs[0].Status)
	}
}

func TestScheduleWorkerPreDispatchInfrastructureFailureUsesBoundedQueueRetry(t *testing.T) {
	d, execution := persistentScheduleFixture(t)
	transient := errors.New("transient inventory failure")
	reads := 0
	d.AssetStore = scheduleAssetReads{MemoryAssetStore: d.AssetStore.(*persistence.MemoryAssetStore), list: func(ctx context.Context) error {
		deadline, bounded := ctx.Deadline()
		if !bounded || time.Until(deadline) > scheduleLookupTimeout {
			return errors.New("inventory read has no bounded deadline")
		}
		reads++
		if reads == 1 {
			return transient
		}
		return nil
	}}
	d.AgentMgr = agentmgr.NewManager()
	d.AgentMgr.Register(agentmgr.NewAgentConn(nil, "asset-1", "linux"))
	calls := 0
	d.ExecuteViaAgent = func(job terminal.CommandJob) terminal.CommandResult {
		calls++
		return terminal.CommandResult{JobID: job.JobID, SessionID: job.SessionID, CommandID: job.CommandID, Status: actions.StatusSucceeded, CompletedAt: time.Now().UTC()}
	}
	var processed atomic.Uint64
	job := scheduleQueueJob(t, execution)
	if err := d.HandleScheduleRunJob(&processed)(context.Background(), job); !errors.Is(err, transient) {
		t.Fatalf("first delivery = %v", err)
	}
	requireScheduleState(t, d, execution, "queued", 0)
	if processed.Load() != 0 || calls != 0 || job.Status == jobqueue.StatusCompleted {
		t.Fatal("failed preparation consumed the occurrence")
	}
	job.Attempts = 2
	if err := d.HandleScheduleRunJob(&processed)(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	requireScheduleState(t, d, execution, "succeeded", 1)
	if calls != 1 || processed.Load() != 1 || job.Status != jobqueue.StatusCompleted {
		t.Fatalf("successful retry calls=%d processed=%d state=%s", calls, processed.Load(), job.Status)
	}
	job.Attempts = 3
	if err := d.HandleScheduleRunJob(&processed)(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	requireScheduleState(t, d, execution, "succeeded", 1)
	if calls != 1 {
		t.Fatal("finalized delivery replayed the command")
	}
	for _, change := range []string{"disabled", "superseded"} {
		t.Run(change+"-during-resolution", func(t *testing.T) {
			d, execution := persistentScheduleFixture(t)
			var expected schedules.ScheduledTask
			d.AssetStore = scheduleAssetReads{MemoryAssetStore: d.AssetStore.(*persistence.MemoryAssetStore), list: func(ctx context.Context) error {
				if change == "disabled" {
					enabled := false
					if err := d.ScheduleStore.UpdateScheduledTask(ctx, execution.ScheduleID, nil, nil, nil, nil, nil, &enabled, schedules.NextRunUpdate{}); err != nil {
						return err
					}
				} else {
					task, _, err := d.ScheduleStore.GetScheduledTask(ctx, execution.ScheduleID)
					if err != nil {
						return err
					}
					task.LastRunJobID = "new-occurrence"
					if err := d.ScheduleExecutionStore.(*persistedScheduleExecution).replace(ctx, task); err != nil {
						return err
					}
				}
				var err error
				expected, _, err = d.ScheduleStore.GetScheduledTask(ctx, execution.ScheduleID)
				return err
			}}
			var processed atomic.Uint64
			job := scheduleQueueJob(t, execution)
			if err := d.HandleScheduleRunJob(&processed)(context.Background(), job); err != nil {
				t.Fatal(err)
			}
			current, _, err := d.ScheduleStore.GetScheduledTask(context.Background(), execution.ScheduleID)
			runs, listErr := d.ActionStore.ListActionRuns(10, 0, "", "")
			if err != nil || listErr != nil || len(runs) != 0 || current.LastRunStatus != expected.LastRunStatus || current.LastRunJobID != expected.LastRunJobID || job.Status != jobqueue.StatusCompleted {
				t.Fatalf("changed definition was not fenced: task=%+v expected=%+v runs=%+v errors=%v/%v", current, expected, runs, err, listErr)
			}
		})
	}
}

func TestScheduleWorkerReadCancellationPreservesDispatchBoundary(t *testing.T) {
	for _, phase := range []string{"assets", "groups", "maintenance-parent"} {
		t.Run(phase, func(t *testing.T) {
			d, execution := persistentScheduleFixture(t)
			// The read barrier models an in-flight store wait; actual Memory data
			// supplies inventory and state. Store lock/pool cancellation has its
			// separate real-owner tests; this checks the worker's context wiring.
			entered := make(chan bool, 1)
			block := func(ctx context.Context) error {
				deadline, ok := ctx.Deadline()
				entered <- ok && time.Until(deadline) > 0 && time.Until(deadline) <= scheduleLookupTimeout
				<-ctx.Done()
				return ctx.Err()
			}
			assetStore := d.AssetStore.(*persistence.MemoryAssetStore)
			switch phase {
			case "assets":
				d.AssetStore = scheduleAssetReads{MemoryAssetStore: assetStore, list: block}
			case "groups":
				d.GroupStore = scheduleGroupReads{MemoryGroupStore: d.GroupStore.(*persistence.MemoryGroupStore), read: block}
			case "maintenance-parent":
				if _, err := assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{AssetID: "asset-1", Metadata: map[string]string{"agent_id": "host-1"}}); err != nil {
					t.Fatal(err)
				}
				guard := &groupfeatures.Deps{AssetStore: scheduleAssetReads{MemoryAssetStore: assetStore, get: func(ctx context.Context, id string) error {
					if id == "host-1" {
						return block(ctx)
					}
					return nil
				}}}
				d.EvaluateAssetGuardrailsContext = guard.EvaluateAssetGuardrailsContext
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			var processed atomic.Uint64
			done := make(chan error, 1)
			job := scheduleQueueJob(t, execution)
			go func() { done <- d.HandleScheduleRunJob(&processed)(ctx, job) }()
			joined := false
			defer func() {
				cancel()
				if !joined {
					select {
					case <-done:
					case <-time.After(3 * time.Second):
						t.Error("worker did not join")
					}
				}
			}()
			select {
			case bounded := <-entered:
				if !bounded {
					t.Fatal("read did not inherit a bounded context")
				}
			case <-time.After(time.Second):
				t.Fatal("worker did not reach the selected read")
			}
			cancel()
			select {
			case err := <-done:
				joined = true
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("canceled worker = %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("cancellation did not return promptly")
			}
			want := "queued"
			if phase == "maintenance-parent" {
				want = "running" // Conservatively unknown; a retry cannot replay.
			}
			requireScheduleState(t, d, execution, want, 0)
			if processed.Load() != 0 {
				t.Fatal("canceled attempt was reported complete")
			}
		})
	}
}

type scheduleMaintenanceReads struct {
	persistence.GroupMaintenanceStore
	group string
}

func (s scheduleMaintenanceReads) ListGroupMaintenanceWindowsContext(ctx context.Context, id string, _ *time.Time, _ int) ([]groupmaintenance.MaintenanceWindow, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if id != s.group {
		return nil, errors.New("wrong inherited maintenance group")
	}
	return []groupmaintenance.MaintenanceWindow{{BlockActions: true}}, nil
}

func TestScheduleWorkerContextGuardrailsHonorInheritedMaintenance(t *testing.T) {
	d, execution := persistentScheduleFixture(t)
	for _, heartbeat := range []assets.HeartbeatRequest{
		{AssetID: "asset-1", Metadata: map[string]string{"agent_id": "host-1"}},
		{AssetID: "host-1", GroupID: "maintenance-group"},
	} {
		if _, err := d.AssetStore.UpsertAssetHeartbeat(heartbeat); err != nil {
			t.Fatal(err)
		}
	}
	guard := &groupfeatures.Deps{AssetStore: d.AssetStore, GroupMaintenanceStore: scheduleMaintenanceReads{group: "maintenance-group"}}
	d.EvaluateAssetGuardrailsContext = guard.EvaluateAssetGuardrailsContext
	var processed atomic.Uint64
	job := scheduleQueueJob(t, execution)
	if err := d.HandleScheduleRunJob(&processed)(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	requireScheduleState(t, d, execution, "blocked", 0)
	if processed.Load() != 1 || job.Status != jobqueue.StatusCompleted {
		t.Fatal("maintenance block was not a terminal outcome")
	}
}

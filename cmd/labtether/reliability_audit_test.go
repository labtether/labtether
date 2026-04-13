package main

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/groupmaintenance"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/updates"
)

type fixedMaintenanceStore struct {
	active map[string][]groupmaintenance.MaintenanceWindow
}

func (f *fixedMaintenanceStore) CreateGroupMaintenanceWindow(groupID string, req groupmaintenance.CreateMaintenanceWindowRequest) (groupmaintenance.MaintenanceWindow, error) {
	return groupmaintenance.MaintenanceWindow{}, nil
}

func (f *fixedMaintenanceStore) GetGroupMaintenanceWindow(groupID, windowID string) (groupmaintenance.MaintenanceWindow, bool, error) {
	return groupmaintenance.MaintenanceWindow{}, false, nil
}

func (f *fixedMaintenanceStore) ListGroupMaintenanceWindows(groupID string, activeAt *time.Time, limit int) ([]groupmaintenance.MaintenanceWindow, error) {
	windows := f.active[groupID]
	if activeAt == nil {
		return append([]groupmaintenance.MaintenanceWindow(nil), windows...), nil
	}
	at := activeAt.UTC()
	out := make([]groupmaintenance.MaintenanceWindow, 0, len(windows))
	for _, window := range windows {
		if !window.StartAt.After(at) && window.EndAt.After(at) {
			out = append(out, window)
		}
	}
	return out, nil
}

func (f *fixedMaintenanceStore) UpdateGroupMaintenanceWindow(groupID, windowID string, req groupmaintenance.UpdateMaintenanceWindowRequest) (groupmaintenance.MaintenanceWindow, error) {
	return groupmaintenance.MaintenanceWindow{}, nil
}

func (f *fixedMaintenanceStore) DeleteGroupMaintenanceWindow(groupID, windowID string) error {
	return nil
}

type recordingReliabilityHistoryStore struct {
	records []struct {
		groupID string
		score   int
		grade   string
	}
}

func (r *recordingReliabilityHistoryStore) InsertReliabilityRecord(groupID string, score int, grade string, factors map[string]any, windowHours int) error {
	r.records = append(r.records, struct {
		groupID string
		score   int
		grade   string
	}{groupID: groupID, score: score, grade: grade})
	return nil
}

func (r *recordingReliabilityHistoryStore) ListReliabilityHistory(groupID string, days int) ([]persistence.ReliabilityRecord, error) {
	return []persistence.ReliabilityRecord{}, nil
}

func (r *recordingReliabilityHistoryStore) PruneReliabilityHistory(olderThanDays int) (int64, error) {
	return 0, nil
}

func TestStatusAggregateIncludesGroupReliability(t *testing.T) {
	sut := newTestAPIServer(t)
	group := mustCreateGroupWithParent(t, sut, "Status Reliability Group", "")

	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID:  "status-reliability-node",
		Type:     "host",
		Name:     "Status Reliability Node",
		Source:   "agent",
		GroupID:  group.ID,
		Status:   "online",
		Platform: "linux",
	}); err != nil {
		t.Fatalf("failed to create asset heartbeat: %v", err)
	}

	response := sut.buildStatusAggregateResponse(context.Background(), "")
	if len(response.GroupReliability) != 1 {
		t.Fatalf("expected one reliability row, got %+v", response.GroupReliability)
	}
	if response.GroupReliability[0].Group.ID != group.ID {
		t.Fatalf("expected reliability row for %q, got %+v", group.ID, response.GroupReliability[0].Group)
	}
}

func TestBuildGroupReliabilityRecordCountsBusyWindowsExactly(t *testing.T) {
	sut := newTestAPIServer(t)
	group := mustCreateGroupWithParent(t, sut, "Busy Reliability Group", "")
	now := time.Now().UTC()

	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID:  "busy-reliability-node",
		Type:     "host",
		Name:     "Busy Reliability Node",
		Source:   "agent",
		GroupID:  group.ID,
		Status:   "online",
		Platform: "linux",
	}); err != nil {
		t.Fatalf("failed to create asset heartbeat: %v", err)
	}

	for index := 0; index < 1101; index++ {
		if err := sut.logStore.AppendEvent(actionsUpdatesTestLogEvent("busy-log-"+itoa(index), "busy-reliability-node", now.Add(-2*time.Minute))); err != nil {
			t.Fatalf("append log event %d: %v", index, err)
		}
	}

	for index := 0; index < 520; index++ {
		run, err := sut.actionStore.CreateActionRun(actions.ExecuteRequest{
			Target:  "busy-reliability-node",
			Command: "echo test",
		})
		if err != nil {
			t.Fatalf("create action run %d: %v", index, err)
		}
		if err := sut.actionStore.ApplyActionResult(actions.Result{
			RunID:       run.ID,
			Status:      actions.StatusFailed,
			Error:       "boom",
			CompletedAt: now.Add(-time.Minute),
		}); err != nil {
			t.Fatalf("apply action result %d: %v", index, err)
		}
	}

	plan, err := sut.updateStore.CreateUpdatePlan(updates.CreatePlanRequest{
		Name:    "Busy Reliability Plan",
		Targets: []string{"busy-reliability-node"},
	})
	if err != nil {
		t.Fatalf("create update plan: %v", err)
	}
	for index := 0; index < 530; index++ {
		run, err := sut.updateStore.CreateUpdateRun(plan, updates.ExecutePlanRequest{})
		if err != nil {
			t.Fatalf("create update run %d: %v", index, err)
		}
		if err := sut.updateStore.ApplyUpdateResult(updates.Result{
			RunID:       run.ID,
			Status:      updates.StatusFailed,
			Error:       "boom",
			CompletedAt: now.Add(-30 * time.Second),
		}); err != nil {
			t.Fatalf("apply update result %d: %v", index, err)
		}
	}

	record, err := sut.ensureGroupFeaturesDeps().BuildGroupReliabilityRecord(group, now.Add(-time.Hour), now)
	if err != nil {
		t.Fatalf("BuildGroupReliabilityRecord failed: %v", err)
	}
	if record.ErrorLogs != 1101 {
		t.Fatalf("expected 1101 error logs, got %d", record.ErrorLogs)
	}
	if record.FailedActions != 520 {
		t.Fatalf("expected 520 failed actions, got %d", record.FailedActions)
	}
	if record.FailedUpdates != 530 {
		t.Fatalf("expected 530 failed updates, got %d", record.FailedUpdates)
	}
}

func TestBuildGroupReliabilityRecordUsesHistoricalMaintenanceWindow(t *testing.T) {
	sut := newTestAPIServer(t)
	group := mustCreateGroupWithParent(t, sut, "Historical Maintenance Group", "")
	windowStart := time.Date(2024, time.January, 2, 10, 0, 0, 0, time.UTC)
	windowEnd := windowStart.Add(2 * time.Hour)
	sut.groupMaintenanceStore = &fixedMaintenanceStore{
		active: map[string][]groupmaintenance.MaintenanceWindow{
			group.ID: []groupmaintenance.MaintenanceWindow{
				{
					ID:             "maint-1",
					GroupID:        group.ID,
					Name:           "Historical Window",
					StartAt:        windowStart,
					EndAt:          windowEnd,
					SuppressAlerts: true,
					BlockActions:   true,
					BlockUpdates:   true,
				},
			},
		},
	}
	sut.groupFeaturesDeps = nil
	sut.groupFeaturesDepsOnce = sync.Once{}

	record, err := sut.ensureGroupFeaturesDeps().BuildGroupReliabilityRecord(
		group,
		windowStart.Add(-time.Hour),
		windowStart.Add(30*time.Minute),
	)
	if err != nil {
		t.Fatalf("BuildGroupReliabilityRecord failed: %v", err)
	}
	if !record.MaintenanceActive || !record.BlockActions || !record.BlockUpdates || !record.SuppressAlerts {
		t.Fatalf("expected historical maintenance guardrails to be active, got %+v", record)
	}
}

func TestMaterializeReliabilityWritesRecords(t *testing.T) {
	sut := newTestAPIServer(t)
	group := mustCreateGroupWithParent(t, sut, "Materialized Reliability Group", "")
	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID:  "materialized-reliability-node",
		Type:     "host",
		Name:     "Materialized Reliability Node",
		Source:   "agent",
		GroupID:  group.ID,
		Status:   "online",
		Platform: "linux",
	}); err != nil {
		t.Fatalf("failed to create asset heartbeat: %v", err)
	}

	history := &recordingReliabilityHistoryStore{}
	sut.ensureGroupFeaturesDeps().MaterializeReliability(history)
	if len(history.records) != 1 {
		t.Fatalf("expected one materialized reliability record, got %+v", history.records)
	}
	if history.records[0].groupID != group.ID {
		t.Fatalf("expected materialized group %q, got %+v", group.ID, history.records[0])
	}
}

func actionsUpdatesTestLogEvent(id, assetID string, at time.Time) logs.Event {
	return logs.Event{
		ID:        id,
		AssetID:   assetID,
		Source:    "agent",
		Level:     "error",
		Message:   "busy window",
		Timestamp: at,
	}
}

func itoa(value int) string {
	return strconv.Itoa(value)
}

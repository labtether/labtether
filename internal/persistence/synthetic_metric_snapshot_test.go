package persistence

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/synthetic"
)

func assertSyntheticMetricSnapshotStore(t *testing.T, store SyntheticStore, snapshotStore SyntheticMetricSnapshotStore) {
	t.Helper()
	enabled := true
	check, err := store.CreateSyntheticCheck(synthetic.CreateCheckRequest{
		Name: "snapshot-check", CheckType: synthetic.CheckTypeHTTP,
		Target: "https://user:password@example.test/?token=secret", Enabled: &enabled,
	})
	if err != nil {
		t.Fatalf("create synthetic check: %v", err)
	}
	t.Cleanup(func() { _ = store.DeleteSyntheticCheck(check.ID) })

	olderLatency := 99
	if _, err := store.RecordSyntheticResult(check.ID, synthetic.Result{
		Status: synthetic.ResultStatusFail, LatencyMS: &olderLatency,
		CheckedAt: time.Now().UTC().Add(23 * time.Hour),
	}); err != nil {
		t.Fatalf("record older result: %v", err)
	}
	latestLatency := 12
	latest, err := store.RecordSyntheticResult(check.ID, synthetic.Result{
		Status: synthetic.ResultStatusOK, LatencyMS: &latestLatency,
		CheckedAt: time.Now().UTC().Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("record latest result: %v", err)
	}
	outOfOrderLatency := 77
	if _, err := store.RecordSyntheticResult(check.ID, synthetic.Result{
		Status: synthetic.ResultStatusFail, LatencyMS: &outOfOrderLatency,
		CheckedAt: latest.CheckedAt.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("record out-of-order result: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	snapshots, err := snapshotStore.LatestSyntheticMetricSnapshots(ctx, 500)
	if err != nil {
		t.Fatalf("latest synthetic metric snapshots: %v", err)
	}
	var found *SyntheticMetricSnapshot
	for index := range snapshots {
		if snapshots[index].CheckID == check.ID {
			found = &snapshots[index]
			break
		}
	}
	if found == nil {
		t.Fatalf("created check missing from snapshots: %+v", snapshots)
	}
	if found.ResultID != latest.ID || found.Status != synthetic.ResultStatusOK || found.LatencyMS == nil || *found.LatencyMS != latestLatency || found.CheckedAt != latest.CheckedAt {
		t.Fatalf("latest synthetic metric snapshot = %+v", *found)
	}

	canceled, cancelNow := context.WithCancel(context.Background())
	cancelNow()
	if _, err := snapshotStore.LatestSyntheticMetricSnapshots(canceled, 500); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled synthetic snapshot error = %v, want context.Canceled", err)
	}
}

func TestMemorySyntheticMetricSnapshotStore(t *testing.T) {
	store := NewMemorySyntheticStore()
	assertSyntheticMetricSnapshotStore(t, store, store)
}

func TestPostgresSyntheticMetricSnapshotStore(t *testing.T) {
	store := newTestPostgresStore(t)
	assertSyntheticMetricSnapshotStore(t, store, store)
}

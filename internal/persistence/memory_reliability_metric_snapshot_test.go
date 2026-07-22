package persistence

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/groups"
	"github.com/labtether/labtether/internal/telemetry"
)

func TestMemoryReliabilityMetricSnapshotsAreLatestFreshAndDeterministicallyBounded(t *testing.T) {
	now := time.Now().UTC()
	store := NewMemoryGroupStore()
	store.groups = map[string]groups.Group{
		"group-a": {ID: "group-a", Name: "Alpha"},
		"group-b": {ID: "group-b", Name: "Beta"},
		"group-c": {ID: "group-c", Name: "Gamma"},
	}
	store.reliabilityHistory = map[string][]ReliabilityRecord{
		"group-a": {
			{ID: "rh-old", GroupID: "group-a", Score: 10, ComputedAt: now.Add(-25 * time.Hour)},
			{ID: "rh-a", GroupID: "group-a", Score: 80, ComputedAt: now.Add(-time.Hour)},
			{ID: "rh-z", GroupID: "group-a", Score: 90, ComputedAt: now.Add(-time.Hour)},
			{ID: "rh-future", GroupID: "group-a", Score: 100, ComputedAt: now.Add(time.Hour)},
		},
		"group-b": {{ID: "rh-b", GroupID: "group-b", Score: 70, ComputedAt: now.Add(-2 * time.Hour)}},
		"group-c": {{ID: "rh-c", GroupID: "group-c", Score: 60, ComputedAt: now.Add(-3 * time.Hour)}},
	}

	got, err := store.LatestReliabilityMetricSnapshots(context.Background(), now, 2)
	if err != nil {
		t.Fatalf("latest reliability snapshots: %v", err)
	}
	want := []ReliabilityMetricSnapshot{
		{GroupID: "group-a", GroupName: "Alpha", Score: 90},
		{GroupID: "group-b", GroupName: "Beta", Score: 70},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("snapshots = %+v, want %+v", got, want)
	}

	// The group cap applies before history lookup, matching the PostgreSQL CTE;
	// stale/empty groups cannot make the scan move on to a later ID.
	store.reliabilityHistory["group-a"] = []ReliabilityRecord{{ID: "stale", GroupID: "group-a", Score: 1, ComputedAt: now.Add(-25 * time.Hour)}}
	got, err = store.LatestReliabilityMetricSnapshots(context.Background(), now, 2)
	if err != nil {
		t.Fatalf("latest reliability snapshots with stale group: %v", err)
	}
	if !reflect.DeepEqual(got, []ReliabilityMetricSnapshot{{GroupID: "group-b", GroupName: "Beta", Score: 70}}) {
		t.Fatalf("stale/cap semantics mismatch: %+v", got)
	}
}

func TestMemoryReliabilityMetricSnapshotsValidateLimitAndContext(t *testing.T) {
	store := NewMemoryGroupStore()
	if _, err := store.LatestReliabilityMetricSnapshots(context.Background(), time.Now(), telemetry.MaxSiteReliabilityMetricSeries+1); !errors.Is(err, ErrReliabilityMetricSnapshotLimitExceeded) {
		t.Fatalf("limit error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.LatestReliabilityMetricSnapshots(ctx, time.Now(), telemetry.MaxSiteReliabilityMetricSeries); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled snapshot error = %v", err)
	}
}

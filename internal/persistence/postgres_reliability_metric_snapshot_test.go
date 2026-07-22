package persistence

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/groups"
	"github.com/labtether/labtether/internal/telemetry"
)

func TestPostgresReliabilityMetricSnapshotsLatestFreshAndBounded(t *testing.T) {
	store := newTestPostgresStore(t)
	now := time.Now().UTC()
	suffix := fmt.Sprintf("%d", now.UnixNano())
	groupA, err := store.CreateGroup(groups.CreateRequest{Name: "LTQA reliability A " + suffix})
	if err != nil {
		t.Fatalf("create group A: %v", err)
	}
	groupB, err := store.CreateGroup(groups.CreateRequest{Name: "LTQA reliability B " + suffix})
	if err != nil {
		t.Fatalf("create group B: %v", err)
	}
	t.Cleanup(func() {
		_, _ = store.pool.Exec(context.Background(), `DELETE FROM groups WHERE id = ANY($1::text[])`, []string{groupA.ID, groupB.ID})
	})

	for _, fixture := range []struct {
		id      string
		groupID string
		score   int
		at      time.Time
	}{
		{id: "ltqa-rh-stale-" + suffix, groupID: groupA.ID, score: 10, at: now.Add(-25 * time.Hour)},
		{id: "ltqa-rh-a-" + suffix, groupID: groupA.ID, score: 80, at: now.Add(-time.Hour)},
		{id: "ltqa-rh-z-" + suffix, groupID: groupA.ID, score: 90, at: now.Add(-time.Hour)},
		{id: "ltqa-rh-future-" + suffix, groupID: groupA.ID, score: 100, at: now.Add(time.Hour)},
	} {
		if _, err := store.pool.Exec(context.Background(), `
			INSERT INTO group_reliability_history (id, group_id, score, grade, factors, window_hours, computed_at)
			VALUES ($1, $2, $3, 'A', '{}'::jsonb, 24, $4)`, fixture.id, fixture.groupID, fixture.score, fixture.at); err != nil {
			t.Fatalf("insert reliability fixture %q: %v", fixture.id, err)
		}
	}

	snapshots, err := store.LatestReliabilityMetricSnapshots(context.Background(), now, telemetry.MaxSiteReliabilityMetricSeries)
	if err != nil {
		t.Fatalf("latest reliability snapshots: %v", err)
	}
	var foundA bool
	for _, snapshot := range snapshots {
		if snapshot.GroupID == groupB.ID {
			t.Fatalf("group without history unexpectedly exported: %+v", snapshot)
		}
		if snapshot.GroupID == groupA.ID {
			foundA = true
			if snapshot.GroupName != groupA.Name || snapshot.Score != 90 {
				t.Fatalf("latest/tie/freshness mismatch: %+v", snapshot)
			}
		}
	}
	if !foundA {
		t.Fatalf("group %q missing from reliability snapshot", groupA.ID)
	}

	if _, err := store.LatestReliabilityMetricSnapshots(context.Background(), now, telemetry.MaxSiteReliabilityMetricSeries+1); !errors.Is(err, ErrReliabilityMetricSnapshotLimitExceeded) {
		t.Fatalf("limit error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.LatestReliabilityMetricSnapshots(ctx, now, telemetry.MaxSiteReliabilityMetricSeries); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled snapshot error = %v", err)
	}
}

package main

import (
	"context"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/groupfailover"
	"github.com/labtether/labtether/internal/groups"
	"github.com/labtether/labtether/internal/persistence"
)

type failoverReadinessStoreStub struct {
	pair    groupfailover.FailoverPair
	updates chan int
}

func (s *failoverReadinessStoreStub) CreateFailoverPair(groupfailover.CreatePairRequest) (groupfailover.FailoverPair, error) {
	return s.pair, nil
}
func (s *failoverReadinessStoreStub) GetFailoverPair(id string) (groupfailover.FailoverPair, bool, error) {
	return s.pair, id == s.pair.ID, nil
}
func (s *failoverReadinessStoreStub) ListFailoverPairs(int) ([]groupfailover.FailoverPair, error) {
	return []groupfailover.FailoverPair{s.pair}, nil
}
func (s *failoverReadinessStoreStub) UpdateFailoverPair(string, groupfailover.UpdatePairRequest) (groupfailover.FailoverPair, error) {
	return s.pair, nil
}
func (s *failoverReadinessStoreStub) DeleteFailoverPair(string) error { return nil }
func (s *failoverReadinessStoreStub) UpdateFailoverReadiness(_ string, score int, _ time.Time) error {
	select {
	case s.updates <- score:
	default:
	}
	return nil
}

func TestRunFailoverReadinessCheckerChecksImmediately(t *testing.T) {
	groupStore := persistence.NewMemoryGroupStore()
	primary, err := groupStore.CreateGroup(groups.CreateRequest{Name: "Primary", Slug: "primary"})
	if err != nil {
		t.Fatalf("create primary group: %v", err)
	}
	backup, err := groupStore.CreateGroup(groups.CreateRequest{Name: "Backup", Slug: "backup"})
	if err != nil {
		t.Fatalf("create backup group: %v", err)
	}
	assetStore := persistence.NewMemoryAssetStore()
	if _, err := assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "backup-node-1",
		Type:    "device",
		Name:    "Backup node",
		Source:  "test",
		GroupID: backup.ID,
		Status:  "online",
	}); err != nil {
		t.Fatalf("create backup asset: %v", err)
	}

	store := &failoverReadinessStoreStub{
		pair: groupfailover.FailoverPair{
			ID:             "pair-1",
			PrimaryGroupID: primary.ID,
			BackupGroupID:  backup.ID,
		},
		updates: make(chan int, 1),
	}
	srv := &apiServer{failoverStore: store, groupStore: groupStore, assetStore: assetStore}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.runFailoverReadinessChecker(ctx)
	}()

	select {
	case score := <-store.updates:
		if score != 100 {
			t.Fatalf("readiness score = %d, want 100", score)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("readiness checker did not run immediately")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("readiness checker did not stop after cancellation")
	}
}

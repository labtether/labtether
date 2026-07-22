package resources

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/groupfailover"
	"github.com/labtether/labtether/internal/groups"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/servicehttp"
)

type manualFailoverStoreStub struct {
	pair        groupfailover.FailoverPair
	updateScore int
	getCalls    int
}

func (s *manualFailoverStoreStub) CreateFailoverPair(groupfailover.CreatePairRequest) (groupfailover.FailoverPair, error) {
	return s.pair, nil
}
func (s *manualFailoverStoreStub) GetFailoverPair(id string) (groupfailover.FailoverPair, bool, error) {
	s.getCalls++
	return s.pair, id == s.pair.ID, nil
}
func (s *manualFailoverStoreStub) ListFailoverPairs(int) ([]groupfailover.FailoverPair, error) {
	return []groupfailover.FailoverPair{s.pair}, nil
}
func (s *manualFailoverStoreStub) UpdateFailoverPair(string, groupfailover.UpdatePairRequest) (groupfailover.FailoverPair, error) {
	return s.pair, nil
}
func (s *manualFailoverStoreStub) DeleteFailoverPair(string) error { return nil }
func (s *manualFailoverStoreStub) UpdateFailoverReadiness(_ string, score int, _ time.Time) error {
	s.updateScore = score
	return nil
}

func failoverReadinessStores(t *testing.T) (*persistence.MemoryGroupStore, *persistence.MemoryAssetStore, groupfailover.FailoverPair) {
	t.Helper()
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
	for index := 0; index < 2; index++ {
		if _, err := assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
			AssetID: "backup-offline-" + string(rune('a'+index)),
			Type:    "device",
			Name:    "Offline backup node",
			Source:  "test",
			GroupID: backup.ID,
			Status:  "offline",
		}); err != nil {
			t.Fatalf("create backup asset: %v", err)
		}
	}
	return groupStore, assetStore, groupfailover.FailoverPair{
		ID:             "pair-1",
		PrimaryGroupID: primary.ID,
		BackupGroupID:  backup.ID,
	}
}

func TestManualFailoverReadinessUsesScheduledScoringContract(t *testing.T) {
	groupStore, assetStore, pair := failoverReadinessStores(t)
	store := &manualFailoverStoreStub{pair: pair}
	deps := &Deps{
		FailoverStore: store,
		GroupStore:    groupStore,
		AssetStore:    assetStore,
		EnforceRateLimit: func(http.ResponseWriter, *http.Request, string, int, time.Duration) bool {
			return true
		},
	}
	req := httptest.NewRequest(http.MethodPost, "/group-failover-pairs/pair-1/check-readiness", nil)
	rec := httptest.NewRecorder()
	deps.HandleFailoverPairActions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if store.updateScore != 80 {
		t.Fatalf("stored readiness score = %d, want 80", store.updateScore)
	}
	var response struct {
		ReadinessScore int `json:"readiness_score"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.ReadinessScore != 80 {
		t.Fatalf("response score = %d, want 80", response.ReadinessScore)
	}
}

func TestManualFailoverReadinessAppliesAdmissionBeforeStoreReads(t *testing.T) {
	groupStore, assetStore, pair := failoverReadinessStores(t)
	store := &manualFailoverStoreStub{pair: pair}
	deps := &Deps{
		FailoverStore: store,
		GroupStore:    groupStore,
		AssetStore:    assetStore,
		EnforceRateLimit: func(w http.ResponseWriter, _ *http.Request, _ string, _ int, _ time.Duration) bool {
			servicehttp.WriteError(w, http.StatusTooManyRequests, "rate limit exceeded")
			return false
		},
	}
	req := httptest.NewRequest(http.MethodPost, "/group-failover-pairs/pair-1/check-readiness", nil)
	rec := httptest.NewRecorder()
	deps.HandleFailoverPairActions(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429: %s", rec.Code, rec.Body.String())
	}
	if store.getCalls != 0 {
		t.Fatalf("store reads = %d, want zero before denied admission", store.getCalls)
	}
}

func TestLoadFailoverReadinessSnapshotFailsClosedWithoutCompleteInventory(t *testing.T) {
	if _, err := LoadFailoverReadinessSnapshot(nil, persistence.NewMemoryAssetStore()); err == nil {
		t.Fatal("expected missing group store to fail")
	}
	if _, err := LoadFailoverReadinessSnapshot(persistence.NewMemoryGroupStore(), nil); err == nil {
		t.Fatal("expected missing asset store to fail")
	}
}

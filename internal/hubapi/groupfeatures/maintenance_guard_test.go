package groupfeatures

import (
	"testing"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/groupmaintenance"
	"github.com/labtether/labtether/internal/persistence"
)

type maintenanceGuardStore struct {
	windowsByGroup map[string][]groupmaintenance.MaintenanceWindow
}

func (s *maintenanceGuardStore) CreateGroupMaintenanceWindow(string, groupmaintenance.CreateMaintenanceWindowRequest) (groupmaintenance.MaintenanceWindow, error) {
	return groupmaintenance.MaintenanceWindow{}, nil
}

func (s *maintenanceGuardStore) GetGroupMaintenanceWindow(string, string) (groupmaintenance.MaintenanceWindow, bool, error) {
	return groupmaintenance.MaintenanceWindow{}, false, nil
}

func (s *maintenanceGuardStore) ListGroupMaintenanceWindows(groupID string, activeAt *time.Time, limit int) ([]groupmaintenance.MaintenanceWindow, error) {
	windows := append([]groupmaintenance.MaintenanceWindow(nil), s.windowsByGroup[groupID]...)
	if activeAt == nil {
		return windows, nil
	}
	filtered := make([]groupmaintenance.MaintenanceWindow, 0, len(windows))
	for _, window := range windows {
		if !window.StartAt.After(*activeAt) && window.EndAt.After(*activeAt) {
			filtered = append(filtered, window)
		}
	}
	if limit > 0 && len(filtered) > limit {
		filtered = filtered[:limit]
	}
	return filtered, nil
}

func (s *maintenanceGuardStore) UpdateGroupMaintenanceWindow(string, string, groupmaintenance.UpdateMaintenanceWindowRequest) (groupmaintenance.MaintenanceWindow, error) {
	return groupmaintenance.MaintenanceWindow{}, nil
}

func (s *maintenanceGuardStore) DeleteGroupMaintenanceWindow(string, string) error { return nil }

func TestEvaluateAssetGuardrailsResolvesDirectAndDerivedAssetGroups(t *testing.T) {
	now := time.Now().UTC()
	assetStore := persistence.NewMemoryAssetStore()
	_, err := assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "host-1",
		GroupID: "group-1",
	})
	if err != nil {
		t.Fatalf("seed host: %v", err)
	}
	_, err = assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID:  "docker-ct-host-1-abc",
		Metadata: map[string]string{"agent_id": "host-1"},
	})
	if err != nil {
		t.Fatalf("seed derived asset: %v", err)
	}

	deps := &Deps{
		AssetStore: assetStore,
		GroupMaintenanceStore: &maintenanceGuardStore{windowsByGroup: map[string][]groupmaintenance.MaintenanceWindow{
			"group-1": {{
				ID:             "window-1",
				GroupID:        "group-1",
				StartAt:        now.Add(-time.Minute),
				EndAt:          now.Add(time.Minute),
				SuppressAlerts: true,
				BlockActions:   true,
			}},
		}},
	}

	for _, assetID := range []string{"host-1", "docker-ct-host-1-abc"} {
		guardrails, evalErr := deps.EvaluateAssetGuardrails(assetID, now)
		if evalErr != nil {
			t.Fatalf("evaluate %s: %v", assetID, evalErr)
		}
		if guardrails.GroupID != "group-1" || !guardrails.BlockActions || !guardrails.SuppressAlerts || len(guardrails.ActiveWindows) != 1 {
			t.Fatalf("unexpected guardrails for %s: %+v", assetID, guardrails)
		}
	}
}

func TestEvaluateAssetGuardrailsAllowsUnknownAndUngroupedAssets(t *testing.T) {
	assetStore := persistence.NewMemoryAssetStore()
	if _, err := assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{AssetID: "ungrouped"}); err != nil {
		t.Fatalf("seed asset: %v", err)
	}
	deps := &Deps{AssetStore: assetStore, GroupMaintenanceStore: &maintenanceGuardStore{}}
	for _, assetID := range []string{"missing", "ungrouped"} {
		guardrails, err := deps.EvaluateAssetGuardrails(assetID, time.Now())
		if err != nil {
			t.Fatalf("evaluate %s: %v", assetID, err)
		}
		if guardrails.BlockActions || guardrails.SuppressAlerts || guardrails.GroupID != "" {
			t.Fatalf("unexpected guardrails for %s: %+v", assetID, guardrails)
		}
	}
}

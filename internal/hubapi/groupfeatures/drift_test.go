package groupfeatures

import (
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/groupprofiles"
)

func TestComputeGroupDriftDetectsExpectedAssetCount(t *testing.T) {
	status, details := ComputeGroupDrift(groupprofiles.Profile{
		Config: map[string]any{"expected_asset_count": "1"},
	}, []assets.Asset{
		{ID: "asset-1"},
		{ID: "asset-2"},
	})

	if status != groupprofiles.DriftStatusDrifted {
		t.Fatalf("status=%q, want drifted", status)
	}
	if got := details["drifted_fields"]; got != 1 {
		t.Fatalf("drifted_fields=%v, want 1", got)
	}
}

func TestComputeGroupDriftTreatsInvalidNumericProfileConfigAsDrift(t *testing.T) {
	status, details := ComputeGroupDrift(groupprofiles.Profile{
		Config: map[string]any{
			"expected_asset_count": "2x",
			"min_online_percent":   "1e3",
		},
	}, []assets.Asset{{ID: "asset-1", Status: "online"}})

	if status != groupprofiles.DriftStatusDrifted {
		t.Fatalf("status=%q, want drifted", status)
	}
	reasons, ok := details["reasons"].([]string)
	if !ok {
		t.Fatalf("reasons=%T, want []string", details["reasons"])
	}
	if !containsReason(reasons, "expected_asset_count") {
		t.Fatalf("expected invalid expected_asset_count reason, got %v", reasons)
	}
	if !containsReason(reasons, "min_online_percent") {
		t.Fatalf("expected invalid min_online_percent reason, got %v", reasons)
	}
}

func TestComputeGroupDriftHonorsZeroExpectedAssets(t *testing.T) {
	status, details := ComputeGroupDrift(groupprofiles.Profile{
		Config: map[string]any{"expected_asset_count": 0},
	}, []assets.Asset{{ID: "asset-1"}})

	if status != groupprofiles.DriftStatusDrifted {
		t.Fatalf("status=%q, want drifted", status)
	}
	reasons, ok := details["reasons"].([]string)
	if !ok || !containsReason(reasons, "expected 0 assets") {
		t.Fatalf("expected zero asset-count drift reason, got %v", details["reasons"])
	}
}

func containsReason(reasons []string, needle string) bool {
	for _, reason := range reasons {
		if strings.Contains(reason, needle) {
			return true
		}
	}
	return false
}

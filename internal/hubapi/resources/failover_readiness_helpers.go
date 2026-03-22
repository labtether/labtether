package resources

// failover_readiness_helpers.go — pure functions for computing failover readiness
// scores. The runner methods that call these live in cmd/labtether/failover_readiness.go.

import (
	"strings"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/groupfailover"
	"github.com/labtether/labtether/internal/groups"
)

// FailoverReadinessAssetCounts holds asset totals for a single group.
type FailoverReadinessAssetCounts struct {
	Total  int
	Online int
}

// FailoverReadinessSnapshot is the pre-loaded state used by ComputeFailoverReadiness.
type FailoverReadinessSnapshot struct {
	GroupsByID         map[string]groups.Group
	GroupsLoaded       bool
	AssetCountsByGroup map[string]FailoverReadinessAssetCounts
	AssetsLoaded       bool
}

// FailoverAssetCountsByGroup builds a map of group ID → asset counts from a flat asset list.
func FailoverAssetCountsByGroup(assetList []assets.Asset) map[string]FailoverReadinessAssetCounts {
	countsByGroup := make(map[string]FailoverReadinessAssetCounts, len(assetList))
	for _, assetEntry := range assetList {
		groupID := strings.TrimSpace(assetEntry.GroupID)
		if groupID == "" {
			continue
		}
		counts := countsByGroup[groupID]
		counts.Total++
		if strings.EqualFold(strings.TrimSpace(assetEntry.Status), "online") {
			counts.Online++
		}
		countsByGroup[groupID] = counts
	}
	return countsByGroup
}

// ComputeFailoverReadiness evaluates how ready a backup group is to take over.
// Group readiness is based on group existence and asset health only; the group
// model no longer carries a separate activation-status field.
func ComputeFailoverReadiness(pair groupfailover.FailoverPair, snapshot FailoverReadinessSnapshot) int {
	score := 100

	if _, ok := snapshot.GroupsByID[strings.TrimSpace(pair.BackupGroupID)]; !ok {
		return 0
	}

	if _, ok := snapshot.GroupsByID[strings.TrimSpace(pair.PrimaryGroupID)]; !ok {
		score -= 10
	}

	if snapshot.AssetsLoaded {
		backupCounts := snapshot.AssetCountsByGroup[strings.TrimSpace(pair.BackupGroupID)]
		if backupCounts.Total == 0 {
			score -= 30
		} else if backupCounts.Online*2 < backupCounts.Total {
			score -= 20
		}
	}

	if score < 0 {
		score = 0
	}
	return score
}

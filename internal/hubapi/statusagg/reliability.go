package statusagg

import (
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/groups"
	groupfeatures "github.com/labtether/labtether/internal/hubapi/groupfeatures"
)

func (d *Deps) listGroupReliability(
	groupList []groups.Group,
	assetList []assets.Asset,
) []groupfeatures.GroupReliabilityRecord {
	if len(groupList) == 0 || d.GroupStore == nil || d.AssetStore == nil || d.LogStore == nil || d.ActionStore == nil || d.UpdateStore == nil {
		return []groupfeatures.GroupReliabilityRecord{}
	}

	deps := groupfeatures.Deps{
		GroupStore:            d.GroupStore,
		AssetStore:            d.AssetStore,
		LogStore:              d.LogStore,
		ActionStore:           d.ActionStore,
		UpdateStore:           d.UpdateStore,
		GroupMaintenanceStore: d.GroupMaintenanceStore,
	}

	now := time.Now().UTC()
	records, err := deps.BuildGroupReliabilityRecordsWithAssets(groupList, assetList, now.Add(-24*time.Hour), now)
	if err != nil {
		logf("failed to build group reliability: %v", err)
		return []groupfeatures.GroupReliabilityRecord{}
	}
	return records
}

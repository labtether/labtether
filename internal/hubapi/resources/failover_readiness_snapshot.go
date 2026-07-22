package resources

import (
	"fmt"
	"strings"

	"github.com/labtether/labtether/internal/groups"
	"github.com/labtether/labtether/internal/persistence"
)

// LoadFailoverReadinessSnapshot loads one bounded, internally consistent view
// for both scheduled and operator-triggered readiness checks. Missing or
// failed stores are errors: scoring without either groups or assets can
// otherwise inflate readiness and present a false-safe result.
func LoadFailoverReadinessSnapshot(
	groupStore persistence.GroupStore,
	assetStore persistence.AssetStore,
) (FailoverReadinessSnapshot, error) {
	if groupStore == nil || assetStore == nil {
		return FailoverReadinessSnapshot{}, fmt.Errorf("failover readiness stores unavailable")
	}

	groupList, err := groupStore.ListGroups()
	if err != nil {
		return FailoverReadinessSnapshot{}, fmt.Errorf("load failover readiness groups: %w", err)
	}
	assetList, err := assetStore.ListAssets()
	if err != nil {
		return FailoverReadinessSnapshot{}, fmt.Errorf("load failover readiness assets: %w", err)
	}

	snapshot := FailoverReadinessSnapshot{
		GroupsByID:         make(map[string]groups.Group, len(groupList)),
		GroupsLoaded:       true,
		AssetCountsByGroup: FailoverAssetCountsByGroup(assetList),
		AssetsLoaded:       true,
	}
	for _, groupEntry := range groupList {
		groupID := strings.TrimSpace(groupEntry.ID)
		if groupID != "" {
			snapshot.GroupsByID[groupID] = groupEntry
		}
	}
	return snapshot, nil
}

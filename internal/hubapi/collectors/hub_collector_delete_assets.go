package collectors

import (
	"errors"
	"strings"

	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/persistence"
)

// deleteCollectorAssets removes the devices a removed connector found: assets
// in its ID scope, or older unscoped ones naming it in collector_id. Agent
// devices are never removed here. Left behind, these devices kept pointing
// at a connection that no longer exists.
func (d *Deps) deleteCollectorAssets(collectorID string) (int, error) {
	collectorID = strings.TrimSpace(collectorID)
	if d.AssetStore == nil || collectorID == "" {
		return 0, nil
	}
	list, err := d.AssetStore.ListAssets()
	if err != nil {
		return 0, err
	}
	scope := assetid.CollectorScope(collectorID)
	removed := 0
	for _, asset := range list {
		if !collectorOwnsAsset(asset, collectorID, scope) {
			continue
		}
		if err := d.AssetStore.DeleteAsset(asset.ID); err != nil && !errors.Is(err, persistence.ErrNotFound) {
			return removed, err
		}
		removed++
	}
	return removed, nil
}

func collectorOwnsAsset(asset assets.Asset, collectorID, scope string) bool {
	if strings.EqualFold(strings.TrimSpace(asset.Source), "agent") {
		return false
	}
	if assetScope, scoped := assetid.CollectorScopeFromAssetID(asset.ID); scoped {
		return assetScope == scope
	}
	return strings.TrimSpace(asset.Metadata["collector_id"]) == collectorID
}

package collectors

import (
	"strings"

	"github.com/labtether/labtether/internal/hubcollector"
)

// fallbackClusterName names a collector's group asset when its config has no
// cluster or display name. An auto-provisioned Docker collector never has
// one, so its group asset was named "" and showed an empty label in the
// console and Home Assistant. It is named after the agent's device; any other
// collector keeps the name its asset has, else its ID.
func (d *Deps) fallbackClusterName(collector hubcollector.Collector, source string) string {
	if agentID := CollectorConfigString(collector.Config, "agent_asset_id"); agentID != "" && strings.EqualFold(source, "docker") {
		return "Docker on " + d.assetName(agentID)
	}
	if name := d.assetName(collector.AssetID); name != strings.TrimSpace(collector.AssetID) {
		return name
	}
	return strings.TrimSpace(collector.AssetID)
}

// assetName returns the asset's name, or its ID when it has none.
func (d *Deps) assetName(assetID string) string {
	assetID = strings.TrimSpace(assetID)
	if d.AssetStore == nil || assetID == "" {
		return assetID
	}
	if asset, ok, err := d.AssetStore.GetAsset(assetID); err == nil && ok && strings.TrimSpace(asset.Name) != "" {
		return strings.TrimSpace(asset.Name)
	}
	return assetID
}

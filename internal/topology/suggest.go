package topology

// Suggestion represents a placement suggestion for an unsorted asset.
type Suggestion struct {
	AssetID   string `json:"asset_id"`
	ZoneID    string `json:"zone_id,omitempty"`
	ZoneLabel string `json:"zone_label,omitempty"`
	Reason    string `json:"reason,omitempty"` // "parent_host", "same_source", "same_type"
}

// SuggestPlacements generates placement suggestions for unsorted assets.
// Priority (first match wins):
//  1. Parent host already in a zone → suggest same zone
//  2. Same source as majority of existing zone members → suggest that zone
//  3. Same type as majority of existing zone members → suggest that zone
//  4. No match → no suggestion (empty ZoneID)
func SuggestPlacements(
	unsortedAssets []AssetInfo,
	zones []Zone,
	members []ZoneMember,
	memberAssets map[string]AssetInfo, // assetID → AssetInfo for assets already in zones
	parentMap map[string]string, // childAssetID → parentAssetID from contains edges
) []Suggestion {
	if len(unsortedAssets) == 0 {
		return nil
	}

	// Build membersByZone: zoneID → []AssetInfo
	membersByZone := make(map[string][]AssetInfo, len(zones))
	for _, m := range members {
		if info, ok := memberAssets[m.AssetID]; ok {
			membersByZone[m.ZoneID] = append(membersByZone[m.ZoneID], info)
		}
	}

	// Build a lookup from assetID → zoneID for assets already placed.
	assetZone := make(map[string]string)
	for _, m := range members {
		assetZone[m.AssetID] = m.ZoneID
	}

	// Build zoneLabel lookup: zoneID → label
	zoneLabel := make(map[string]string, len(zones))
	for _, z := range zones {
		zoneLabel[z.ID] = z.Label
	}

	suggestions := make([]Suggestion, 0, len(unsortedAssets))

	for _, asset := range unsortedAssets {
		sug := Suggestion{AssetID: asset.ID}

		// Priority 1: parent host in a zone.
		if parentID, ok := parentMap[asset.ID]; ok && parentID != "" {
			if zID, placed := assetZone[parentID]; placed {
				sug.ZoneID = zID
				sug.ZoneLabel = zoneLabel[zID]
				sug.Reason = "parent_host"
				suggestions = append(suggestions, sug)
				continue
			}
		}

		// Priority 2 & 3: majority match on source then type across zones.
		bestZoneID, bestReason := findMajorityZone(asset, zones, membersByZone)
		if bestZoneID != "" {
			sug.ZoneID = bestZoneID
			sug.ZoneLabel = zoneLabel[bestZoneID]
			sug.Reason = bestReason
		}

		suggestions = append(suggestions, sug)
	}

	return suggestions
}

// findMajorityZone returns the zone where the majority of members share the
// asset's source (checked first) or type (checked second).
// Returns ("", "") if no zone qualifies.
func findMajorityZone(asset AssetInfo, zones []Zone, membersByZone map[string][]AssetInfo) (string, string) {
	// Try source match first.
	if zoneID := bestZoneByField(asset.Source, "source", zones, membersByZone, func(a AssetInfo) string { return a.Source }); zoneID != "" {
		return zoneID, "same_source"
	}
	// Try type match.
	if zoneID := bestZoneByField(asset.Type, "type", zones, membersByZone, func(a AssetInfo) string { return a.Type }); zoneID != "" {
		return zoneID, "same_type"
	}
	return "", ""
}

// bestZoneByField returns the zone ID whose members have a strict majority
// sharing the given value for the extracted field. If multiple zones tie on
// majority fraction, the zone with the higher absolute count wins; ties are
// broken by zone order (first in the zones slice).
func bestZoneByField(
	value string,
	_ string,
	zones []Zone,
	membersByZone map[string][]AssetInfo,
	field func(AssetInfo) string,
) string {
	if value == "" {
		return ""
	}

	bestZoneID := ""
	bestCount := 0
	bestTotal := 0

	for _, z := range zones {
		zm := membersByZone[z.ID]
		if len(zm) == 0 {
			continue
		}
		match := 0
		for _, m := range zm {
			if field(m) == value {
				match++
			}
		}
		// Require strict majority (> 50%).
		if match*2 <= len(zm) {
			continue
		}
		// Prefer higher match count; on tie prefer earlier zone (first wins).
		if match > bestCount || (match == bestCount && len(zm) < bestTotal) {
			bestCount = match
			bestTotal = len(zm)
			bestZoneID = z.ID
		}
	}

	return bestZoneID
}

// DismissedAssetState tracks an asset's state at dismissal time for change detection.
type DismissedAssetState struct {
	TopologyID string
	AssetID    string
	Source     string
	Type       string
}

// CheckDismissedForChanges compares dismissed assets against their current state.
// Returns asset IDs that should be undismissed because their source or type changed.
func CheckDismissedForChanges(
	dismissed []DismissedAssetState,
	currentAssets map[string]AssetInfo,
) []string {
	var changed []string
	for _, d := range dismissed {
		current, exists := currentAssets[d.AssetID]
		if !exists {
			continue // asset deleted, leave dismissed
		}
		if current.Source != d.Source || current.Type != d.Type {
			changed = append(changed, d.AssetID)
		}
	}
	return changed
}

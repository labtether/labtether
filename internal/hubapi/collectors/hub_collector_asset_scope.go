package collectors

import (
	"fmt"
	"strings"

	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/assets"
)

const (
	collectorNativeAssetIDMetadataKey = "collector_native_asset_id"
	collectorScopeMetadataKey         = "collector_scope"
)

// ProcessScopedCollectorHeartbeat persists a connector-discovered asset under
// a collector-scoped ID. This prevents independent connector instances that
// reuse native IDs (VMIDs, endpoint IDs, pool names, or entity IDs) from
// overwriting one another in the global asset store.
//
// A legacy unscoped asset is adopted only when its collector_id proves that it
// belongs to this collector. User-managed group, tags, and name override are
// carried forward before the legacy row is removed. An unowned or differently
// owned legacy row is deliberately left untouched.
func (d *Deps) ProcessScopedCollectorHeartbeat(collectorID string, req assets.HeartbeatRequest) (*assets.Asset, error) {
	collectorID = strings.TrimSpace(collectorID)
	nativeID := assetid.NativeCollectorAssetID(req.AssetID)
	if collectorID == "" || nativeID == "" {
		return d.ProcessHeartbeatRequest(req)
	}

	req = ScopedCollectorHeartbeatRequest(collectorID, req)

	var legacy assets.Asset
	adoptLegacy := false
	if d.AssetStore != nil && nativeID != req.AssetID {
		if candidate, ok, err := d.AssetStore.GetAsset(nativeID); err != nil {
			return nil, fmt.Errorf("load legacy collector asset %s: %w", nativeID, err)
		} else if ok && strings.TrimSpace(candidate.Metadata["collector_id"]) == collectorID {
			legacy = candidate
			adoptLegacy = true
			if strings.TrimSpace(req.GroupID) == "" {
				req.GroupID = strings.TrimSpace(candidate.GroupID)
			}
			if nameOverride := strings.TrimSpace(candidate.Metadata[assets.MetadataKeyNameOverride]); nameOverride != "" {
				req.Metadata[assets.MetadataKeyNameOverride] = nameOverride
			}
		}
	}

	stored, err := d.ProcessHeartbeatRequest(req)
	if err != nil {
		return nil, err
	}
	if !adoptLegacy || d.AssetStore == nil {
		return stored, nil
	}

	var update assets.UpdateRequest
	if nameOverride := strings.TrimSpace(legacy.Metadata[assets.MetadataKeyNameOverride]); nameOverride != "" {
		update.Name = &nameOverride
	}
	if len(legacy.Tags) > 0 {
		tags := append([]string(nil), legacy.Tags...)
		update.Tags = &tags
	}
	if update.Name != nil || update.Tags != nil {
		updated, updateErr := d.AssetStore.UpdateAsset(req.AssetID, update)
		if updateErr != nil {
			return nil, fmt.Errorf("adopt legacy collector asset customizations for %s: %w", nativeID, updateErr)
		}
		stored = &updated
	}
	if err := d.AssetStore.DeleteAsset(nativeID); err != nil {
		return nil, fmt.Errorf("remove adopted legacy collector asset %s: %w", nativeID, err)
	}
	return stored, nil
}

// ScopedCollectorHeartbeatRequest applies collector identity to a heartbeat
// before both persistence and canonical snapshot synthesis.
func ScopedCollectorHeartbeatRequest(collectorID string, req assets.HeartbeatRequest) assets.HeartbeatRequest {
	collectorID = strings.TrimSpace(collectorID)
	nativeID := assetid.NativeCollectorAssetID(req.AssetID)
	if collectorID == "" || nativeID == "" {
		return req
	}

	if req.Metadata == nil {
		req.Metadata = make(map[string]string, 3)
	} else {
		// Metadata cardinality originates at connector boundaries. Avoid deriving
		// an allocation size (or doing overflow-prone capacity arithmetic) from it;
		// these maps are deliberately small and can grow normally while cloning.
		cloned := make(map[string]string, 3)
		for key, value := range req.Metadata {
			cloned[key] = value
		}
		req.Metadata = cloned
	}
	req.Metadata["collector_id"] = collectorID
	req.Metadata[collectorNativeAssetIDMetadataKey] = nativeID
	req.Metadata[collectorScopeMetadataKey] = assetid.CollectorScope(collectorID)
	req.AssetID = assetid.ScopeCollectorAssetID(nativeID, collectorID)
	return req
}

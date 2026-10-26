package persistence

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/assets"
)

// TrueNASActionAssetStore resolves the IDs that TrueNAS actually consumes.
// At most two rows are returned: callers must reject an ambiguous inventory.
// Unowned legacy rows are included so they cannot silently bypass a lock.
type TrueNASActionAssetStore interface {
	FindTrueNASActionAssets(collectorID, assetType, vmID string) ([]assets.Asset, error)
}

func validateTrueNASActionLookup(assetType, vmID string) error {
	if (assetType == "vm" && vmID != "") || (assetType == "nas" && vmID == "") {
		return nil
	}
	return fmt.Errorf("invalid TrueNAS action asset lookup")
}

func (s *PostgresStore) FindTrueNASActionAssets(collectorID, assetType, vmID string) ([]assets.Asset, error) {
	if err := validateTrueNASActionLookup(assetType, vmID); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.listAssets(ctx,
		`SELECT id, type, name, source, group_id, status, platform, metadata, tags, created_at, updated_at, last_seen_at, host, transport_type
		 FROM assets
		 WHERE source = 'truenas' AND type = $1
		   AND (BTRIM(COALESCE(metadata->>'collector_id', '')) = $2
		        OR (BTRIM(COALESCE(metadata->>'collector_id', '')) = ''
		            AND (id !~ '--ltc-[0-9a-fA-F]{32}$' OR ($2 <> '' AND RIGHT(id, 38) = '--ltc-' || $4))))
		   AND ($3 = '' OR metadata->>'vm_id' = $3)
		 ORDER BY id LIMIT 2`, assetType, strings.TrimSpace(collectorID), vmID, assetid.CollectorScope(collectorID))
}

func (m *MemoryAssetStore) FindTrueNASActionAssets(collectorID, assetType, vmID string) ([]assets.Asset, error) {
	if err := validateTrueNASActionLookup(assetType, vmID); err != nil {
		return nil, err
	}
	collectorID = strings.TrimSpace(collectorID)
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]assets.Asset, 0, 2)
	for _, asset := range m.assets {
		owner := strings.TrimSpace(asset.Metadata["collector_id"])
		if asset.Source != "truenas" || asset.Type != assetType || (owner != collectorID && owner != "") ||
			(vmID != "" && asset.Metadata["vm_id"] != vmID) {
			continue
		}
		if owner == "" && collectorID != "" {
			if scope, scoped := assetid.CollectorScopeFromAssetID(asset.ID); scoped && scope != assetid.CollectorScope(collectorID) {
				continue
			}
		}
		asset = applyAssetCanonical(asset)
		asset.Tags = cloneStringSlice(asset.Tags)
		asset.Metadata = cloneMetadata(asset.Metadata)
		asset.Attributes = cloneAnyMap(asset.Attributes)
		out = append(out, asset)
		if len(out) == 2 {
			break
		}
	}
	return out, nil
}

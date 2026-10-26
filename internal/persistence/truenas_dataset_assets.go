package persistence

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/assets"
)

// TrueNASDatasetAssetStore pages existing owners, not provider task names.
// Ambiguous legacy identities are returned so the caller can fail closed.
type TrueNASDatasetAssetStore interface {
	ListTrueNASDatasetAssets(context.Context, string, string, string, int) ([]assets.Asset, error)
}

func validateTrueNASDatasetPage(root string, limit int) error {
	if root == "" || len(root) > 4096 || limit < 1 || limit > 256 {
		return fmt.Errorf("invalid TrueNAS dataset page")
	}
	return nil
}

func (s *PostgresStore) ListTrueNASDatasetAssets(ctx context.Context, collectorID, root, after string, limit int) ([]assets.Asset, error) {
	if err := validateTrueNASDatasetPage(root, limit); err != nil {
		return nil, err
	}
	return s.listAssets(ctx, `SELECT id, type, name, source, group_id, status, platform,
	 metadata, tags, created_at, updated_at, last_seen_at, host, transport_type
	 FROM assets WHERE source = 'truenas' AND type = 'dataset' AND id COLLATE "C" > $3
	 AND (BTRIM(COALESCE(metadata->>'collector_id','')) = $1
	      OR (BTRIM(COALESCE(metadata->>'collector_id','')) = ''
	          AND (id !~ '--ltc-[0-9a-fA-F]{32}$' OR ($1 <> '' AND RIGHT(id,38) = '--ltc-' || $5))))
	 AND (COALESCE(NULLIF(metadata->>'dataset_name',''),name) = $2
	      OR LEFT(COALESCE(NULLIF(metadata->>'dataset_name',''),name),LENGTH($2)+1) = $2 || '/'
	      OR (COALESCE(metadata->>'dataset_name','') = '' AND COALESCE(metadata->>'name_override','') <> ''))
	 ORDER BY id COLLATE "C" LIMIT $4`, strings.TrimSpace(collectorID), root, after, limit, assetid.CollectorScope(collectorID))
}

func (m *MemoryAssetStore) ListTrueNASDatasetAssets(ctx context.Context, collectorID, root, after string, limit int) ([]assets.Asset, error) {
	if err := validateTrueNASDatasetPage(root, limit); err != nil {
		return nil, err
	}
	collectorID = strings.TrimSpace(collectorID)
	m.mu.RLock()
	defer m.mu.RUnlock()
	// Keep only the next page in memory, even for a large inventory.
	out := make([]assets.Asset, 0, limit)
	for _, a := range m.assets {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		owner := strings.TrimSpace(a.Metadata["collector_id"])
		if a.Source != "truenas" || a.Type != "dataset" || a.ID <= after || (owner != collectorID && owner != "") {
			continue
		}
		if owner == "" && collectorID != "" {
			if scope, scoped := assetid.CollectorScopeFromAssetID(a.ID); scoped && scope != assetid.CollectorScope(collectorID) {
				continue
			}
		}
		name := a.Metadata["dataset_name"]
		ambiguous := name == "" && a.Metadata[assets.MetadataKeyNameOverride] != ""
		if name == "" {
			name = a.Name
		}
		if !ambiguous && name != root && !strings.HasPrefix(name, root+"/") {
			continue
		}
		index := sort.Search(len(out), func(i int) bool { return out[i].ID >= a.ID })
		if index == limit {
			continue
		}
		a = applyAssetCanonical(a)
		a.Tags = cloneStringSlice(a.Tags)
		a.Metadata = cloneMetadata(a.Metadata)
		a.Attributes = cloneAnyMap(a.Attributes)
		out = append(out, a)
		copy(out[index+1:], out[index:len(out)-1])
		out[index] = a
		if len(out) > limit {
			out = out[:limit]
		}
	}
	return out, nil
}

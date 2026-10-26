package persistence

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/assets"
)

// PBSActionAssetStore uses the provider's exact datastore name, not its lossy
// normalized asset ID. A third match exposes ambiguous inventory to the caller.
type PBSActionAssetStore interface {
	FindPBSActionAssets(collectorID, assetType, store string) ([]assets.Asset, error)
}

func validatePBSActionLookup(kind, store string) error {
	if (kind == "storage-controller" && store == "") || (kind == "storage-pool" && store != "") {
		return nil
	}
	return fmt.Errorf("invalid PBS action asset lookup")
}

func (s *PostgresStore) FindPBSActionAssets(collector, kind, store string) ([]assets.Asset, error) {
	if err := validatePBSActionLookup(kind, store); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.listAssets(ctx, `SELECT id, type, name, source, group_id, status, platform,
	 metadata, tags, created_at, updated_at, last_seen_at, host, transport_type
	 FROM assets WHERE source = 'pbs' AND type = $1
	 AND ($2 = '' OR metadata->>'store' = $2)
	 AND (BTRIM(COALESCE(metadata->>'collector_id','')) = $3
	      OR (BTRIM(COALESCE(metadata->>'collector_id','')) = ''
	          AND (id !~ '--ltc-[0-9a-fA-F]{32}$' OR ($3 <> '' AND RIGHT(id,38) = '--ltc-' || $4))))
	 ORDER BY id LIMIT 3`, kind, store, strings.TrimSpace(collector), assetid.CollectorScope(collector))
}

func (m *MemoryAssetStore) FindPBSActionAssets(collector, kind, store string) ([]assets.Asset, error) {
	if err := validatePBSActionLookup(kind, store); err != nil {
		return nil, err
	}
	collector = strings.TrimSpace(collector)
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]assets.Asset, 0, 3)
	for _, a := range m.assets {
		owner := strings.TrimSpace(a.Metadata["collector_id"])
		if a.Source != "pbs" || a.Type != kind || (store != "" && a.Metadata["store"] != store) || (owner != collector && owner != "") {
			continue
		}
		if owner == "" {
			if scope, scoped := assetid.CollectorScopeFromAssetID(a.ID); scoped && scope != assetid.CollectorScope(collector) {
				continue
			}
		}
		a = applyAssetCanonical(a)
		a.Metadata = cloneMetadata(a.Metadata)
		a.Tags = cloneStringSlice(a.Tags)
		a.Attributes = cloneAnyMap(a.Attributes)
		out = append(out, a)
		if len(out) == 3 {
			break
		}
	}
	return out, nil
}

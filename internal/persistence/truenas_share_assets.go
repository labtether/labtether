package persistence

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/assets"
)

// A provider share ID is unique per protocol and collector. Two stored aliases
// (legacy and scoped) may own groups; a third match indicates ambiguity.
type TrueNASShareAssetStore interface {
	FindTrueNASShareAssets(context.Context, string, string, int) ([]assets.Asset, error)
}

func (s *PostgresStore) FindTrueNASShareAssets(ctx context.Context, collector, kind string, id int) ([]assets.Asset, error) {
	if (kind != "smb" && kind != "nfs") || id <= 0 {
		return nil, fmt.Errorf("invalid TrueNAS share identity")
	}
	return s.listAssets(ctx, `SELECT id, type, name, source, group_id, status, platform,
	 metadata, tags, created_at, updated_at, last_seen_at, host, transport_type
	 FROM assets WHERE source = 'truenas' AND type = $1 AND metadata->>'share_id' = $2
	 AND (BTRIM(COALESCE(metadata->>'collector_id','')) = $3
	      OR (BTRIM(COALESCE(metadata->>'collector_id','')) = ''
	          AND (id !~ '--ltc-[0-9a-fA-F]{32}$' OR ($3 <> '' AND RIGHT(id,38) = '--ltc-' || $4))))
	 ORDER BY id LIMIT 3`, "share-"+kind, strconv.Itoa(id), strings.TrimSpace(collector), assetid.CollectorScope(collector))
}

func (m *MemoryAssetStore) FindTrueNASShareAssets(ctx context.Context, collector, kind string, id int) ([]assets.Asset, error) {
	if (kind != "smb" && kind != "nfs") || id <= 0 {
		return nil, fmt.Errorf("invalid TrueNAS share identity")
	}
	collector = strings.TrimSpace(collector)
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]assets.Asset, 0, 3)
	for _, a := range m.assets {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		owner := strings.TrimSpace(a.Metadata["collector_id"])
		if a.Source != "truenas" || a.Type != "share-"+kind || a.Metadata["share_id"] != strconv.Itoa(id) || (owner != collector && owner != "") {
			continue
		}
		if owner == "" && collector != "" {
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

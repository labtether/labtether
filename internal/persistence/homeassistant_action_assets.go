package persistence

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/assets"
)

// HomeAssistantActionAssetStore pages known entity owners, not the provider's
// full physical inventory. Empty entity selects all known entities. Unowned
// legacy rows remain candidates so the caller must prove their ownership.
type HomeAssistantActionAssetStore interface {
	ListHomeAssistantActionAssets(ctx context.Context, collector, entity, after string, limit int) ([]assets.Asset, error)
}

func validateHomeAssistantActionPage(collector, entity, after string, limit int) error {
	if strings.TrimSpace(collector) == "" || len(collector) > 1024 || len(entity) > 1024 || len(after) > 4096 || limit < 1 || limit > 256 {
		return fmt.Errorf("invalid Home Assistant action asset page")
	}
	return nil
}

func (s *PostgresStore) ListHomeAssistantActionAssets(ctx context.Context, collector, entity, after string, limit int) ([]assets.Asset, error) {
	if err := validateHomeAssistantActionPage(collector, entity, after, limit); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return s.listAssets(ctx, `SELECT id, type, name, source, group_id, status, platform,
	 metadata, tags, created_at, updated_at, last_seen_at, host, transport_type
	 FROM assets WHERE LOWER(BTRIM(source)) IN ('homeassistant','home-assistant')
	 AND type = 'ha-entity' AND id COLLATE "C" > $3
	 AND ($2 = '' OR BTRIM(COALESCE(metadata->>'entity_id','')) = $2)
	 AND (BTRIM(COALESCE(metadata->>'collector_id','')) = $1
	      OR (BTRIM(COALESCE(metadata->>'collector_id','')) = ''
	          AND (id !~ '--ltc-[0-9a-fA-F]{32}$' OR LOWER(RIGHT(id,38)) = '--ltc-' || $5)))
	 ORDER BY id COLLATE "C" LIMIT $4`, strings.TrimSpace(collector), entity, after, limit, assetid.CollectorScope(collector))
}

func (m *MemoryAssetStore) ListHomeAssistantActionAssets(ctx context.Context, collector, entity, after string, limit int) ([]assets.Asset, error) {
	if err := validateHomeAssistantActionPage(collector, entity, after, limit); err != nil {
		return nil, err
	}
	collector = strings.TrimSpace(collector)
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]assets.Asset, 0, limit)
	for _, a := range m.assets {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		source, owner := strings.ToLower(strings.TrimSpace(a.Source)), strings.TrimSpace(a.Metadata["collector_id"])
		if (source != "homeassistant" && source != "home-assistant") || a.Type != "ha-entity" || a.ID <= after ||
			(entity != "" && strings.TrimSpace(a.Metadata["entity_id"]) != entity) || (owner != "" && owner != collector) {
			continue
		}
		if owner == "" {
			if scope, scoped := assetid.CollectorScopeFromAssetID(a.ID); scoped && !strings.EqualFold(scope, assetid.CollectorScope(collector)) {
				continue
			}
		}
		index := sort.Search(len(out), func(i int) bool { return out[i].ID >= a.ID })
		if index == limit {
			continue
		}
		a = applyAssetCanonical(a)
		a.Metadata, a.Tags, a.Attributes = cloneMetadata(a.Metadata), cloneStringSlice(a.Tags), cloneAnyMap(a.Attributes)
		out = append(out, a)
		copy(out[index+1:], out[index:len(out)-1])
		out[index] = a
		if len(out) > limit {
			out = out[:limit]
		}
	}
	return out, nil
}

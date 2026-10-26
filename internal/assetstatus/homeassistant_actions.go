package assetstatus

import (
	"context"
	"fmt"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/persistence"
)

func (s *Store) ListHomeAssistantActionAssets(ctx context.Context, collector, entity, after string, limit int) ([]assets.Asset, error) {
	lookup, ok := s.AssetStore.(persistence.HomeAssistantActionAssetStore)
	if !ok {
		return nil, fmt.Errorf("Home Assistant action asset lookup unavailable")
	}
	rows, err := lookup.ListHomeAssistantActionAssets(ctx, collector, entity, after, limit)
	if err != nil {
		return nil, err
	}
	return s.resolveAll(rows), nil
}

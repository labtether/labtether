package assetstatus

import (
	"context"
	"fmt"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/persistence"
)

func (s *Store) ListTrueNASDatasetAssets(ctx context.Context, collectorID, root, after string, limit int) ([]assets.Asset, error) {
	lookup, ok := s.AssetStore.(persistence.TrueNASDatasetAssetStore)
	if !ok {
		return nil, fmt.Errorf("TrueNAS dataset identity lookup unavailable")
	}
	list, err := lookup.ListTrueNASDatasetAssets(ctx, collectorID, root, after, limit)
	if err != nil {
		return nil, err
	}
	return s.resolveAll(list), nil
}

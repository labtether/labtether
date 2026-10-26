package assetstatus

import (
	"context"
	"fmt"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/persistence"
)

func (s *Store) FindTrueNASShareAssets(ctx context.Context, collector, kind string, id int) ([]assets.Asset, error) {
	lookup, ok := s.AssetStore.(persistence.TrueNASShareAssetStore)
	if !ok {
		return nil, fmt.Errorf("TrueNAS share identity lookup unavailable")
	}
	list, err := lookup.FindTrueNASShareAssets(ctx, collector, kind, id)
	if err != nil {
		return nil, err
	}
	return s.resolveAll(list), nil
}

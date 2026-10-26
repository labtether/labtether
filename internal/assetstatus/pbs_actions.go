package assetstatus

import (
	"fmt"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/persistence"
)

func (s *Store) FindPBSActionAssets(collector, kind, store string) ([]assets.Asset, error) {
	lookup, ok := s.AssetStore.(persistence.PBSActionAssetStore)
	if !ok {
		return nil, fmt.Errorf("PBS identity lookup unavailable")
	}
	rows, err := lookup.FindPBSActionAssets(collector, kind, store)
	if err != nil {
		return nil, err
	}
	return s.resolveAll(rows), nil
}

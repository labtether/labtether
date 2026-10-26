// Package assetstatus provides a read-only, runtime-aware view of persisted
// asset status. Persisted heartbeat state survives a hub restart, while active
// agent connections do not, so callers must not treat an old "online" value as
// proof that an agent is connected to the current hub process.
package assetstatus

import (
	"context"
	"fmt"
	"strings"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/persistence"
)

// AgentConnectivity reports whether an asset has an active connection to the
// current hub process.
type AgentConnectivity interface {
	IsConnected(assetID string) bool
}

// Store decorates an AssetStore with a fail-honest read view for agent assets.
// Writes and their returned records are delegated unchanged. List/Get reads
// return copies whose status is offline when no current agent connection exists.
type Store struct {
	persistence.AssetStore
	connectivity AgentConnectivity
}

var (
	_ persistence.AssetStore      = (*Store)(nil)
	_ persistence.GroupAssetStore = (*Store)(nil)
)

// NewStore creates a runtime-aware asset status view.
func NewStore(store persistence.AssetStore, connectivity AgentConnectivity) *Store {
	return &Store{AssetStore: store, connectivity: connectivity}
}

// ListAssets returns the underlying inventory with disconnected agent assets
// represented as offline. It never mutates the records returned by the wrapped
// store.
func (s *Store) ListAssets() ([]assets.Asset, error) {
	assetList, err := s.AssetStore.ListAssets()
	if err != nil {
		return nil, err
	}
	return s.resolveAll(assetList), nil
}

// ListAssetsContext preserves the real context-aware inventory and status view.
func (s *Store) ListAssetsContext(ctx context.Context) ([]assets.Asset, error) {
	store, ok := s.AssetStore.(persistence.AlertAssetContextStore)
	if !ok {
		return nil, fmt.Errorf("context-aware asset inventory unavailable")
	}
	rows, err := store.ListAssetsContext(ctx)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		rows[i] = s.resolve(rows[i])
	}
	return rows, nil
}

// ListAssetsByGroup preserves the optional group-store optimization while
// applying the same runtime status view used by ListAssets.
func (s *Store) ListAssetsByGroup(groupID string) ([]assets.Asset, error) {
	if groupStore, ok := s.AssetStore.(persistence.GroupAssetStore); ok {
		assetList, err := groupStore.ListAssetsByGroup(groupID)
		if err != nil {
			return nil, err
		}
		return s.resolveAll(assetList), nil
	}

	assetList, err := s.AssetStore.ListAssets()
	if err != nil {
		return nil, err
	}
	groupID = strings.TrimSpace(groupID)
	filtered := make([]assets.Asset, 0, len(assetList))
	for _, assetEntry := range assetList {
		if strings.TrimSpace(assetEntry.GroupID) == groupID {
			filtered = append(filtered, s.resolve(assetEntry))
		}
	}
	return filtered, nil
}

// GetAsset returns the runtime status view for one asset.
func (s *Store) GetAsset(id string) (assets.Asset, bool, error) {
	assetEntry, ok, err := s.AssetStore.GetAsset(id)
	if err != nil || !ok {
		return assetEntry, ok, err
	}
	return s.resolve(assetEntry), true, nil
}

// GetAssetContext keeps cancellation through the optional exact-read owner.
// Missing capability is unknown; it must not use the legacy background read.
func (s *Store) GetAssetContext(ctx context.Context, id string) (assets.Asset, bool, error) {
	if err := ctx.Err(); err != nil {
		return assets.Asset{}, false, err
	}
	store, ok := s.AssetStore.(persistence.AssetContextStore)
	if !ok {
		return assets.Asset{}, false, fmt.Errorf("context-aware exact asset lookup unavailable")
	}
	entry, found, err := store.GetAssetContext(ctx, id)
	if err != nil || !found {
		return entry, found, err
	}
	return s.resolve(entry), true, nil
}

func (s *Store) resolveAll(assetList []assets.Asset) []assets.Asset {
	resolved := make([]assets.Asset, len(assetList))
	for i, assetEntry := range assetList {
		resolved[i] = s.resolve(assetEntry)
	}
	return resolved
}

func (s *Store) resolve(assetEntry assets.Asset) assets.Asset {
	if s.connectivity == nil || !strings.EqualFold(strings.TrimSpace(assetEntry.Source), "agent") {
		return assetEntry
	}
	if s.connectivity.IsConnected(assetEntry.ID) {
		return assetEntry
	}
	assetEntry.Status = "offline"
	return assetEntry
}

// FindTrueNASActionAssets preserves the bounded provider-identity lookup through
// the production status decorator. Do not fall back to an unbounded inventory scan.
func (s *Store) FindTrueNASActionAssets(collectorID, assetType, vmID string) ([]assets.Asset, error) {
	lookup, ok := s.AssetStore.(persistence.TrueNASActionAssetStore)
	if !ok {
		return nil, fmt.Errorf("TrueNAS provider identity lookup unavailable")
	}
	list, err := lookup.FindTrueNASActionAssets(collectorID, assetType, vmID)
	if err != nil {
		return nil, err
	}
	return s.resolveAll(list), nil
}

package assetstatus

import (
	"reflect"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/persistence"
)

type staticConnectivity map[string]bool

func (c staticConnectivity) IsConnected(assetID string) bool {
	return c[assetID]
}

type staticAssetStore struct {
	*persistence.MemoryAssetStore
	listed []assets.Asset
}

func (s *staticAssetStore) ListAssets() ([]assets.Asset, error) {
	return append([]assets.Asset(nil), s.listed...), nil
}

func (s *staticAssetStore) GetAsset(id string) (assets.Asset, bool, error) {
	for _, assetEntry := range s.listed {
		if assetEntry.ID == id {
			return assetEntry, true, nil
		}
	}
	return assets.Asset{}, false, nil
}

func TestStoreFreshProcessFailsDisconnectedAgentStatusOffline(t *testing.T) {
	staleSeenAt := time.Date(2026, time.July, 13, 23, 47, 49, 0, time.UTC)
	underlying := &staticAssetStore{
		MemoryAssetStore: persistence.NewMemoryAssetStore(),
		listed: []assets.Asset{
			{
				ID:            "QAWindowsHost",
				Name:          "QAWindowsHost",
				Source:        "agent",
				Status:        "online",
				TransportType: "offline",
				LastSeenAt:    staleSeenAt,
			},
			{
				ID:            "qawindowshost",
				Name:          "qawindowshost",
				Source:        "agent",
				Status:        "online",
				TransportType: "agent",
				LastSeenAt:    staleSeenAt.Add(5 * time.Hour),
			},
			{
				ID:         "pve",
				Name:       "pve",
				Source:     "proxmox",
				Status:     "online",
				LastSeenAt: staleSeenAt,
			},
		},
	}
	original := append([]assets.Asset(nil), underlying.listed...)

	store := NewStore(underlying, staticConnectivity{"qawindowshost": true})
	listed, err := store.ListAssets()
	if err != nil {
		t.Fatalf("ListAssets: %v", err)
	}
	if got := listed[0].Status; got != "offline" {
		t.Fatalf("disconnected stale agent status = %q, want offline", got)
	}
	if got := listed[1].Status; got != "online" {
		t.Fatalf("connected agent status = %q, want persisted online status", got)
	}
	if got := listed[2].Status; got != "online" {
		t.Fatalf("non-agent status = %q, want unchanged online status", got)
	}
	if !reflect.DeepEqual(underlying.listed, original) {
		t.Fatalf("read view mutated persisted records:\n got: %#v\nwant: %#v", underlying.listed, original)
	}

	got, ok, err := store.GetAsset("QAWindowsHost")
	if err != nil || !ok {
		t.Fatalf("GetAsset: ok=%v err=%v", ok, err)
	}
	if got.Status != "offline" {
		t.Fatalf("GetAsset disconnected status = %q, want offline", got.Status)
	}
}

func TestStoreWithoutConnectivityPreservesUnderlyingStatus(t *testing.T) {
	underlying := &staticAssetStore{
		MemoryAssetStore: persistence.NewMemoryAssetStore(),
		listed:           []assets.Asset{{ID: "agent-1", Source: "agent", Status: "online"}},
	}
	store := NewStore(underlying, nil)

	listed, err := store.ListAssets()
	if err != nil {
		t.Fatalf("ListAssets: %v", err)
	}
	if got := listed[0].Status; got != "online" {
		t.Fatalf("status without connectivity authority = %q, want unchanged online", got)
	}
}

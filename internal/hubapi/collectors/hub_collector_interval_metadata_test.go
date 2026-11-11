package collectors

import (
	"testing"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/persistence"
)

// Devices of a 120 s connection flapped "Not responding" because the console
// judged them by the 65 s agent limit. Every device a connection stores must
// carry the connection's interval, the child devices and its own device.
func TestConnectorDevicesCarryTheConnectionInterval(t *testing.T) {
	t.Parallel()
	store := persistence.NewMemoryAssetStore()
	collector := hubcollector.Collector{
		ID:              "collector-interval-120",
		AssetID:         "ha-root-interval",
		CollectorType:   hubcollector.CollectorTypeHomeAssistant,
		Enabled:         true,
		IntervalSeconds: 120,
	}
	d := &Deps{
		AssetStore:        store,
		HubCollectorStore: &homeAssistantCollectorStatusStore{collector: collector},
		ProcessHeartbeatRequest: func(req assets.HeartbeatRequest) (*assets.Asset, error) {
			stored, err := store.UpsertAssetHeartbeat(req)
			return &stored, err
		},
	}

	child, err := d.ProcessScopedCollectorHeartbeat(collector.ID, assets.HeartbeatRequest{
		AssetID: "ha-entity-light-kitchen", Type: "ha-entity", Name: "Kitchen", Source: "homeassistant", Status: "online",
	})
	if err != nil {
		t.Fatalf("store child device: %v", err)
	}
	if got := child.Metadata[CollectorIntervalMetadataKey]; got != "120" {
		t.Fatalf("child device interval = %q, want 120", got)
	}

	if _, ok := d.RefreshCollectorParentAsset(CollectorParentAssetRefreshOptions{
		Collector: collector, Source: "homeassistant", AssetType: "connector-cluster", Name: "ha01",
		Metadata: map[string]string{"connector_type": "homeassistant"},
	}); !ok {
		t.Fatal("store connection device failed")
	}
	root, ok, err := store.GetAsset(collector.AssetID)
	if err != nil || !ok {
		t.Fatalf("load connection device: ok=%v err=%v", ok, err)
	}
	if got := root.Metadata[CollectorIntervalMetadataKey]; got != "120" {
		t.Fatalf("connection device interval = %q, want 120", got)
	}
}

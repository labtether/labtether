package collectors

import (
	"testing"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/persistence"
)

// The auto-provisioned Docker collector's group asset (docker-cluster-<agent>)
// was refreshed with an empty name on every run, so the console and Home
// Assistant showed an empty label for it.
func TestDockerGroupAssetIsNamedAfterItsAgent(t *testing.T) {
	store := persistence.NewMemoryAssetStore()
	if _, err := store.UpsertAssetHeartbeat(assets.HeartbeatRequest{AssetID: "nas01", Name: "nas01", Type: "host", Source: "agent", Status: "online"}); err != nil {
		t.Fatal(err)
	}
	d := &Deps{AssetStore: store, ProcessHeartbeatRequest: func(req assets.HeartbeatRequest) (*assets.Asset, error) {
		asset, err := store.UpsertAssetHeartbeat(req)
		return &asset, err
	}}
	collector := hubcollector.Collector{
		ID: "hc-docker", AssetID: "docker-cluster-nas01", CollectorType: hubcollector.CollectorTypeDocker,
		Config: map[string]any{"provision_mode": "auto", "agent_asset_id": "nas01"},
	}
	if _, ok := d.KeepConnectorClusterAssetAlive(collector, "docker", 3, "test"); !ok {
		t.Fatal("cluster asset not refreshed")
	}
	got, ok, err := store.GetAsset("docker-cluster-nas01")
	if err != nil || !ok || got.Name != "Docker on nas01" {
		t.Fatalf("group asset name %q (found %v, %v), want \"Docker on nas01\"", got.Name, ok, err)
	}
}

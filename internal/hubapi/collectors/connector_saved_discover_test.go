package collectors

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/persistence"
)

// Discover answered an empty list for a connection saved in the console,
// because it read only the connector set by environment variables.
func TestConnectorDiscoverAnswersFromConnectionsSavedInTheConsole(t *testing.T) {
	store := &hubCollectorHandlerStore{collector: hubcollector.Collector{
		ID: "collector-pve", AssetID: "proxmox-cluster-lab", CollectorType: "proxmox", Enabled: true,
	}}
	assetStore := persistence.NewMemoryAssetStore()
	for _, req := range []assets.HeartbeatRequest{
		{AssetID: "proxmox-vm-100", Type: "vm", Name: "web01", Source: "proxmox", Metadata: map[string]string{"collector_id": "collector-pve"}},
		{AssetID: "proxmox-vm-200", Type: "vm", Name: "db01", Source: "proxmox", Metadata: map[string]string{"collector_id": "collector-other"}},
		{AssetID: "nas01", Type: "host", Name: "nas01", Source: "agent", Metadata: map[string]string{"collector_id": "collector-pve"}},
	} {
		if _, err := assetStore.UpsertAssetHeartbeat(req); err != nil {
			t.Fatal(err)
		}
	}
	registry := connectorsdk.NewRegistry()
	registry.Register(unconfiguredHealthConnector{id: "proxmox"})
	registry.Register(unconfiguredHealthConnector{id: "pbs"})
	deps := newHubCollectorHandlerDeps(store)
	deps.ConnectorRegistry = registry
	deps.AssetStore = assetStore

	discover := func(id string) []any {
		rec := httptest.NewRecorder()
		deps.HandleConnectorActions(rec, httptest.NewRequest(http.MethodGet, "/connectors/"+id+"/discover", nil))
		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != http.StatusOK {
			t.Fatalf("%s discover = %d %s", id, rec.Code, rec.Body.String())
		}
		list, _ := body["assets"].([]any)
		return list
	}
	if got := discover("proxmox"); len(got) != 1 || got[0].(map[string]any)["name"] != "web01" {
		t.Fatalf("saved Proxmox connection: discover = %v, want only its own device", got)
	}
	if got := discover("pbs"); len(got) != 0 {
		t.Fatalf("no saved PBS connection: discover = %v, want the environment connector's empty answer", got)
	}
}

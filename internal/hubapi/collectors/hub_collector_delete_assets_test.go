package collectors

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/persistence"
)

// Removing a Portainer connector left its ten devices behind, and each one
// made the Containers page call a connection that no longer existed.
// Removing a connector removes the devices it found and nothing else.
func TestDeleteHubCollectorRemovesOnlyItsDevices(t *testing.T) {
	store := &hubCollectorHandlerStore{collector: hubcollector.Collector{
		ID: "collector-portainer", CollectorType: hubcollector.CollectorTypePortainer, IntervalSeconds: hubcollector.DefaultIntervalSeconds,
	}}
	assetStore := persistence.NewMemoryAssetStore()
	d := newHubCollectorHandlerDeps(store)
	d.AssetStore = assetStore
	seed := func(id, source string, metadata map[string]string) {
		if _, err := assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{AssetID: id, Type: "container", Name: id, Source: source, Metadata: metadata}); err != nil {
			t.Fatal(err)
		}
	}
	scoped := assetid.ScopeCollectorAssetID("portainer-endpoint-1", "collector-portainer")
	seed(scoped, "portainer", nil)
	seed("portainer-container-1-legacy", "portainer", map[string]string{"collector_id": "collector-portainer"})
	otherConnector := assetid.ScopeCollectorAssetID("portainer-endpoint-1", "collector-other")
	seed(otherConnector, "portainer", nil)
	seed("agent-host", "agent", map[string]string{"collector_id": "collector-portainer"})

	rec := httptest.NewRecorder()
	d.HandleHubCollectorActions(rec, httptest.NewRequest(http.MethodDelete, "/hub-collectors/collector-portainer", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", rec.Code, rec.Body.String())
	}
	for id, wantKept := range map[string]bool{scoped: false, "portainer-container-1-legacy": false, otherConnector: true, "agent-host": true} {
		if _, ok, err := assetStore.GetAsset(id); err != nil || ok != wantKept {
			t.Fatalf("asset %s kept=%v err=%v, want kept=%v", id, ok, err, wantKept)
		}
	}
}

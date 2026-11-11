package collectors

import (
	"testing"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/persistence"
)

// Every container Portainer recreated left its old device behind: a stack
// redeployed three times showed four copies of its container.
func TestPortainerCollectorRemovesContainersPortainerNoLongerLists(t *testing.T) {
	store := persistence.NewMemoryAssetStore()
	seed := func(id, typ, collector, endpoint string) {
		t.Helper()
		if _, err := store.UpsertAssetHeartbeat(assets.HeartbeatRequest{AssetID: id, Type: typ, Name: id, Source: "portainer",
			Metadata: map[string]string{"collector_id": collector, "endpoint_id": endpoint}}); err != nil {
			t.Fatal(err)
		}
	}
	seed("ct-live", "container", "hc1", "1")
	seed("ct-old", "container", "hc1", "1")
	seed("stack-old", "stack", "hc1", "1")
	seed("ct-unread-endpoint", "container", "hc1", "2")
	seed("ct-other-connection", "container", "hc2", "1")
	seed("endpoint-1", "container-host", "hc1", "1")

	d := &Deps{AssetStore: store}
	d.prunePortainerAssetsNotSeen("hc1", map[string]struct{}{"ct-live": {}}, map[string]struct{}{"1": {}})

	for id, want := range map[string]bool{"ct-live": true, "ct-old": false, "stack-old": false,
		"ct-unread-endpoint": true, "ct-other-connection": true, "endpoint-1": true} {
		_, ok, err := store.GetAsset(id)
		if err != nil || ok != want {
			t.Errorf("%s kept=%v err=%v, want kept=%v", id, ok, err, want)
		}
	}
}

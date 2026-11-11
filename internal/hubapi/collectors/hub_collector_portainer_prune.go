package collectors

import (
	"errors"
	"log"
	"strings"

	"github.com/labtether/labtether/internal/persistence"
)

// A container Portainer recreates (a stack redeploy, an image pull, a compose
// edit) gets a new ID, and one that is removed is gone; the Portainer
// collector never removed the old devices, so every recreate left a ghost
// container on Devices and Containers. prunePortainerAssetsNotSeen deletes
// this collector's container and stack devices that the run did not list.
// Only endpoints that answered with at least one container or stack are
// pruned: an endpoint Portainer could not read lists nothing, and its devices
// stay (they read as offline) until it answers again.
func (d *Deps) prunePortainerAssetsNotSeen(collectorID string, seen, liveEndpoints map[string]struct{}) {
	if d == nil || d.AssetStore == nil || strings.TrimSpace(collectorID) == "" || len(liveEndpoints) == 0 {
		return
	}
	list, err := d.AssetStore.ListAssets()
	if err != nil {
		log.Printf("hub collector portainer: failed to list devices for the stale check: %v", err)
		return
	}
	for _, candidate := range list {
		if !strings.EqualFold(strings.TrimSpace(candidate.Source), "portainer") {
			continue
		}
		if candidate.Type != "container" && candidate.Type != "stack" {
			continue
		}
		if strings.TrimSpace(candidate.Metadata["collector_id"]) != collectorID {
			continue
		}
		if _, ok := seen[candidate.ID]; ok {
			continue
		}
		if _, ok := liveEndpoints[strings.TrimSpace(candidate.Metadata["endpoint_id"])]; !ok {
			continue
		}
		if err := d.AssetStore.DeleteAsset(candidate.ID); err != nil && !errors.Is(err, persistence.ErrNotFound) {
			log.Printf("hub collector portainer: failed to delete stale device %s: %v", candidate.ID, err)
		}
	}
}

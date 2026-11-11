package resources

import (
	"errors"
	"log"
	"strings"

	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/persistence"
)

// A connection made in Add device has its own device. Removing that device
// (the console's Remove) removed the connection but kept the devices it had
// found outside its direct children, such as a Proxmox node, a TrueNAS host
// or a Portainer environment, and kept its saved login with the secret.
// Both now go with the connection; agent devices and other connections'
// devices never do.

// collectorsOwnedByAsset returns the connections whose own device is assetID.
func (d *Deps) collectorsOwnedByAsset(assetID string) []hubcollector.Collector {
	if d.HubCollectorStore == nil {
		return nil
	}
	collectors, err := d.HubCollectorStore.ListHubCollectors(500, false)
	if err != nil {
		log.Printf("delete device %s: could not list connections: %v", assetID, err)
		return nil
	}
	owned := make([]hubcollector.Collector, 0)
	for _, collector := range collectors {
		if strings.TrimSpace(collector.AssetID) == assetID {
			owned = append(owned, collector)
		}
	}
	return owned
}

// deleteConnectionDevices removes every device the given connections found.
func (d *Deps) deleteConnectionDevices(collectors []hubcollector.Collector, keepID string) error {
	if len(collectors) == 0 {
		return nil
	}
	list, err := d.AssetStore.ListAssets()
	if err != nil {
		return err
	}
	for _, candidate := range list {
		if candidate.ID == keepID || !ownedByAnyConnection(candidate, collectors) {
			continue
		}
		if err := d.AssetStore.DeleteAsset(candidate.ID); err != nil && !errors.Is(err, persistence.ErrNotFound) {
			return err
		}
	}
	return nil
}

func ownedByAnyConnection(candidate assets.Asset, collectors []hubcollector.Collector) bool {
	if strings.EqualFold(strings.TrimSpace(candidate.Source), "agent") {
		return false
	}
	scope, scoped := assetid.CollectorScopeFromAssetID(candidate.ID)
	for _, collector := range collectors {
		if scoped {
			if scope == assetid.CollectorScope(collector.ID) {
				return true
			}
			continue
		}
		if strings.TrimSpace(candidate.Metadata["collector_id"]) == collector.ID {
			return true
		}
	}
	return false
}

// deleteUnusedConnectionLogins removes the saved logins of removed
// connections that nothing else uses any more.
func (d *Deps) deleteUnusedConnectionLogins(collectors []hubcollector.Collector) {
	store, ok := d.CredentialStore.(persistence.CredentialProfileLifecycleStore)
	if !ok {
		return
	}
	for _, collector := range collectors {
		credentialID, _ := collector.Config["credential_id"].(string)
		if credentialID = strings.TrimSpace(credentialID); credentialID == "" {
			continue
		}
		_, err := store.DeleteCredentialProfileIfUnreferenced(credentialID)
		if err != nil && !errors.Is(err, persistence.ErrNotFound) &&
			!errors.Is(err, persistence.ErrCredentialProfileInUse) {
			log.Printf("delete connection %s: saved login %s kept: %v", collector.ID, credentialID, err)
		}
	}
}

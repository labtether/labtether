package resources

import (
	"net/http"
	"strings"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/servicehttp"
)

// AssetDeletePreview says what removing a device also removes, so the Remove
// dialog can name it before anything is deleted. It uses the same selection
// as DeleteAsset.
type AssetDeletePreview struct {
	// Devices counts the other devices removed with it.
	Devices int `json:"devices"`
	// Connection is true when the device is a connection's own device.
	Connection bool `json:"connection"`
	// SavedLogin is true when the connection's saved login goes too, unless
	// something outside the Hub's connections still uses it.
	SavedLogin bool `json:"saved_login"`
}

// HandleAssetDeletePreview answers GET /assets/{id}/delete-preview.
func (d *Deps) HandleAssetDeletePreview(w http.ResponseWriter, r *http.Request, assetEntry assets.Asset) {
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	preview, err := d.PreviewAssetDelete(assetEntry)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "could not work out what the removal includes")
		return
	}
	servicehttp.WriteJSON(w, http.StatusOK, preview)
}

// PreviewAssetDelete lists, without deleting, the devices DeleteAsset would
// remove together with assetEntry.
func (d *Deps) PreviewAssetDelete(assetEntry assets.Asset) (AssetDeletePreview, error) {
	assetID := strings.TrimSpace(assetEntry.ID)
	preview := AssetDeletePreview{}

	clusterIDs := map[string]bool{}
	autoDocker, err := d.autoDockerCollectorsForAsset(assetID)
	if err != nil {
		return preview, err
	}
	for _, collector := range autoDocker {
		if clusterID := strings.TrimSpace(collector.AssetID); clusterID != "" && !strings.EqualFold(clusterID, assetID) {
			clusterIDs[clusterID] = true
		}
	}

	infraParent := IsInfraHostAsset(assetEntry) && NormalizeSource(assetEntry.Source) != "docker"
	owned := d.collectorsOwnedByAsset(assetID)
	list, err := d.AssetStore.ListAssets()
	if err != nil {
		return preview, err
	}
	for _, candidate := range list {
		if candidate.ID == assetID {
			continue
		}
		if clusterIDs[candidate.ID] ||
			(infraParent && InfraChildAttachedToParent(assetEntry, candidate)) ||
			ownedByAnyConnection(candidate, owned) ||
			d.dockerAssetAttachedToAgent(candidate, assetID) {
			preview.Devices++
		}
	}

	preview.Connection = len(owned) > 0
	preview.SavedLogin = d.connectionLoginsGoWith(owned)
	return preview, nil
}

// connectionLoginsGoWith reports whether any of the removed connections has a
// saved login that no other connection uses.
func (d *Deps) connectionLoginsGoWith(owned []hubcollector.Collector) bool {
	if len(owned) == 0 || d.HubCollectorStore == nil {
		return false
	}
	ownedIDs := map[string]bool{}
	logins := map[string]bool{}
	for _, collector := range owned {
		ownedIDs[collector.ID] = true
		if id, _ := collector.Config["credential_id"].(string); strings.TrimSpace(id) != "" {
			logins[strings.TrimSpace(id)] = true
		}
	}
	if len(logins) == 0 {
		return false
	}
	all, err := d.HubCollectorStore.ListHubCollectors(500, false)
	if err != nil {
		return false
	}
	for _, collector := range all {
		if ownedIDs[collector.ID] {
			continue
		}
		if id, _ := collector.Config["credential_id"].(string); logins[strings.TrimSpace(id)] {
			delete(logins, strings.TrimSpace(id))
		}
	}
	return len(logins) > 0
}

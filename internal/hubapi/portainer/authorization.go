package portainer

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/hubapi/shared"
)

// Metadata and scoped IDs must agree. Legacy rows without either owner can
// only belong to the single enabled provider selected by the runtime resolver.
func (d *Deps) portainerAssetOwned(asset assets.Asset, collectorID string) (bool, error) {
	collectorID = strings.TrimSpace(collectorID)
	if collectorID == "" || !strings.EqualFold(strings.TrimSpace(asset.Source), "portainer") {
		return false, fmt.Errorf("Portainer collector identity unavailable")
	}
	owner := strings.TrimSpace(asset.Metadata["collector_id"])
	if scope, scoped := assetid.CollectorScopeFromAssetID(asset.ID); scoped {
		selected := scope == assetid.CollectorScope(collectorID)
		if (selected && owner != "" && owner != collectorID) || (!selected && owner == collectorID) {
			return false, fmt.Errorf("Portainer asset owner is inconsistent")
		}
		return selected, nil
	}
	if owner != "" {
		return owner == collectorID, nil
	}
	unique, err := shared.ResolveEnabledHubCollector(d.HubCollectorStore, "portainer", "")
	if err != nil {
		return false, err
	}
	return unique.ID == collectorID, nil
}

func (d *Deps) portainerEndpointAllowed(ctx context.Context, collectorID, endpointID string) (bool, error) {
	if !shared.HasAssetRestriction(ctx) {
		return true, nil
	}
	if d.AssetStore == nil {
		return false, fmt.Errorf("asset authorization store unavailable")
	}
	assetList, err := d.AssetStore.ListAssets()
	if err != nil {
		return false, err
	}
	endpointID = strings.TrimSpace(endpointID)
	found := false
	for _, asset := range assetList {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		if !strings.EqualFold(strings.TrimSpace(asset.Source), "portainer") || strings.TrimSpace(asset.Metadata["endpoint_id"]) != endpointID {
			continue
		}
		owned, err := d.portainerAssetOwned(asset, collectorID)
		if err != nil {
			return false, err
		}
		if !owned {
			continue
		}
		found = true
		if !apiv2.AssetCheckContext(ctx, asset.ID) {
			return false, nil
		}
	}
	return found, nil
}

func (d *Deps) portainerResourceAllowed(ctx context.Context, collectorID, endpointID, metadataKey, resourceID string) (bool, error) {
	if !shared.HasAssetRestriction(ctx) {
		return true, nil
	}
	if d.AssetStore == nil {
		return false, fmt.Errorf("asset authorization store unavailable")
	}
	assetList, err := d.AssetStore.ListAssets()
	if err != nil {
		return false, err
	}
	endpointID = strings.TrimSpace(endpointID)
	resourceID = strings.TrimSpace(resourceID)
	found := false
	for _, asset := range assetList {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		if !strings.EqualFold(strings.TrimSpace(asset.Source), "portainer") || strings.TrimSpace(asset.Metadata["endpoint_id"]) != endpointID {
			continue
		}
		if strings.TrimSpace(asset.Metadata[metadataKey]) != resourceID {
			continue
		}
		owned, err := d.portainerAssetOwned(asset, collectorID)
		if err != nil {
			return false, err
		}
		if !owned {
			continue
		}
		found = true
		if !apiv2.AssetCheckContext(ctx, asset.ID) {
			return false, nil
		}
	}
	return found, nil
}

func (d *Deps) requirePortainerEndpointAccess(w http.ResponseWriter, r *http.Request, collectorID, endpointID string) bool {
	allowed, err := d.portainerEndpointAllowed(r.Context(), collectorID, endpointID)
	if err == nil && allowed {
		return true
	}
	apiv2.WriteError(w, http.StatusForbidden, "asset_forbidden", "api key does not have access to every asset in this Portainer endpoint")
	return false
}

func (d *Deps) requirePortainerResourceAccess(w http.ResponseWriter, r *http.Request, collectorID, endpointID, metadataKey, resourceID string) bool {
	allowed, err := d.portainerResourceAllowed(r.Context(), collectorID, endpointID, metadataKey, resourceID)
	if err == nil && allowed {
		return true
	}
	apiv2.WriteError(w, http.StatusForbidden, "asset_forbidden", "api key does not have access to this Portainer resource")
	return false
}

package portainer

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/hubapi/shared"
)

func (d *Deps) portainerEndpointAllowed(ctx context.Context, endpointID string) (bool, error) {
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
		if !strings.EqualFold(strings.TrimSpace(asset.Source), "portainer") || strings.TrimSpace(asset.Metadata["endpoint_id"]) != endpointID {
			continue
		}
		found = true
		if !apiv2.AssetCheckContext(ctx, asset.ID) {
			return false, nil
		}
	}
	return found, nil
}

func (d *Deps) portainerResourceAllowed(ctx context.Context, endpointID, metadataKey, resourceID string) (bool, error) {
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
		if !strings.EqualFold(strings.TrimSpace(asset.Source), "portainer") || strings.TrimSpace(asset.Metadata["endpoint_id"]) != endpointID {
			continue
		}
		if strings.TrimSpace(asset.Metadata[metadataKey]) != resourceID {
			continue
		}
		found = true
		if !apiv2.AssetCheckContext(ctx, asset.ID) {
			return false, nil
		}
	}
	return found, nil
}

func (d *Deps) requirePortainerEndpointAccess(w http.ResponseWriter, r *http.Request, endpointID string) bool {
	allowed, err := d.portainerEndpointAllowed(r.Context(), endpointID)
	if err == nil && allowed {
		return true
	}
	apiv2.WriteError(w, http.StatusForbidden, "asset_forbidden", "api key does not have access to every asset in this Portainer endpoint")
	return false
}

func (d *Deps) requirePortainerResourceAccess(w http.ResponseWriter, r *http.Request, endpointID, metadataKey, resourceID string) bool {
	allowed, err := d.portainerResourceAllowed(r.Context(), endpointID, metadataKey, resourceID)
	if err == nil && allowed {
		return true
	}
	apiv2.WriteError(w, http.StatusForbidden, "asset_forbidden", "api key does not have access to this Portainer resource")
	return false
}

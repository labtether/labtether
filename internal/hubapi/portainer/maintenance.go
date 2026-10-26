package portainer

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/hubapi/maintenanceguard"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/servicehttp"
)

// Guard only actual owners of this effect. A sibling container's maintenance
// window does not lock every resource in the endpoint.
func (d *Deps) enforcePortainerMutation(ctx context.Context, w http.ResponseWriter, asset assets.Asset, runtime *PortainerRuntime, endpointID int, key, resourceID string) bool {
	targets, err := d.portainerMutationOwners(&asset, runtime.CollectorID, endpointID, key, resourceID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusConflict, "cannot verify Portainer mutation owners")
		return false
	}
	if ctx.Err() != nil {
		servicehttp.WriteError(w, http.StatusRequestTimeout, "Portainer mutation canceled before dispatch")
		return false
	}
	if err := maintenanceguard.CheckAssetActions(targets, d.EvaluateAssetGuardrails); err != nil {
		if !maintenanceguard.WriteBlockedAction(w, err) {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to evaluate maintenance windows")
		}
		return false
	}
	if ctx.Err() != nil {
		servicehttp.WriteError(w, http.StatusRequestTimeout, "Portainer mutation canceled before dispatch")
		return false
	}
	return true
}

func (d *Deps) portainerMutationOwners(asset *assets.Asset, collectorID string, endpointID int, key, resourceID string) ([]string, error) {
	if d.AssetStore == nil || endpointID <= 0 {
		return nil, fmt.Errorf("Portainer inventory unavailable")
	}
	collector, err := shared.ResolveEnabledHubCollector(d.HubCollectorStore, "portainer", collectorID)
	if err != nil {
		return nil, err
	}
	endpoint := strconv.Itoa(endpointID)
	targets := []string{strings.TrimSpace(collector.AssetID)}
	if asset != nil {
		owned, err := d.portainerAssetOwned(*asset, collector.ID)
		if err != nil || !owned || strings.TrimSpace(asset.Metadata["endpoint_id"]) != endpoint {
			return nil, fmt.Errorf("Portainer loaded asset does not own this endpoint")
		}
		targets = append(targets, asset.ID)
	}
	identities := []struct{ native, kind, key, value string }{{"portainer-endpoint-" + endpoint, "container-host", "", ""}}
	switch key {
	case "":
	case "container_id":
		if !fullPortainerContainerID(resourceID) {
			return nil, fmt.Errorf("Portainer full container identity unavailable")
		}
		identities = append(identities, struct{ native, kind, key, value string }{
			"portainer-container-" + endpoint + "-" + resourceID[:12], "container", key, resourceID})
	case "stack_id":
		id, err := strconv.Atoi(resourceID)
		if err != nil || id <= 0 {
			return nil, fmt.Errorf("Portainer stack identity invalid")
		}
		identities = append(identities, struct{ native, kind, key, value string }{"portainer-stack-" + strconv.Itoa(id), "stack", key, strconv.Itoa(id)})
	default:
		return nil, fmt.Errorf("unknown Portainer resource identity")
	}
	for _, identity := range identities {
		for _, id := range []string{identity.native, assetid.ScopeCollectorAssetID(identity.native, collector.ID)} {
			row, found, err := d.AssetStore.GetAsset(id)
			if err != nil {
				return nil, err
			}
			if !found {
				continue
			}
			owned, err := d.portainerAssetOwned(row, collector.ID)
			if err != nil {
				return nil, err
			}
			if !owned {
				continue
			}
			if row.Type != identity.kind || row.Metadata["endpoint_id"] != endpoint ||
				(identity.key != "" && row.Metadata[identity.key] != identity.value) {
				return nil, fmt.Errorf("Portainer stored resource alias is ambiguous")
			}
			targets = append(targets, row.ID)
		}
	}
	return targets, nil
}

func fullPortainerContainerID(id string) bool {
	if len(id) != 64 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

// Names and short IDs are mutable aliases. Inspect on the captured provider,
// authorize the returned full identity, then use only that ID for the effect.
func (d *Deps) preparePortainerContainerMutation(ctx context.Context, w http.ResponseWriter, r *http.Request, asset assets.Asset, runtime *PortainerRuntime, endpointID int, requested string) (string, bool) {
	data, err := runtime.Client.InspectContainer(ctx, endpointID, requested)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "cannot resolve Portainer container identity")
		return "", false
	}
	var inspected struct {
		ID string `json:"Id"`
	}
	if err := json.Unmarshal(data, &inspected); err != nil || !fullPortainerContainerID(inspected.ID) {
		servicehttp.WriteError(w, http.StatusConflict, "Portainer returned an invalid full container identity")
		return "", false
	}
	if !d.requirePortainerResourceAccess(w, r, runtime.CollectorID, strconv.Itoa(endpointID), "container_id", inspected.ID) ||
		!d.enforcePortainerMutation(ctx, w, asset, runtime, endpointID, "container_id", inspected.ID) {
		return "", false
	}
	return inspected.ID, true
}

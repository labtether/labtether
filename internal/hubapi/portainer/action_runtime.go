package portainer

import (
	"context"
	"fmt"
	"maps"
	"strconv"
	"strings"

	"github.com/labtether/labtether/internal/assets"
	portainerconnector "github.com/labtether/labtether/internal/connectors/portainer"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/hubapi/maintenanceguard"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/modelmap"
)

// ExecuteAction consumes only a previously bound request. It never resolves
// the original container name/short ID again after the action has been queued.
func (d *Deps) ExecuteAction(ctx context.Context, actionID string, req connectorsdk.ActionRequest) (connectorsdk.ActionResult, error) {
	if err := ctx.Err(); err != nil {
		return connectorsdk.ActionResult{}, err
	}
	key, err := portainerActionKey(actionID, req.TargetID)
	if err != nil {
		return connectorsdk.ActionResult{}, err
	}
	savedAsset, bound := req.Params[portainerBoundAsset]
	if !bound || strings.TrimSpace(req.Params["collector_id"]) == "" || strings.TrimSpace(req.Params[key]) == "" || strings.TrimSpace(req.Params["endpoint_id"]) == "" {
		return connectorsdk.ActionResult{}, fmt.Errorf("Portainer action has no saved resource binding; resubmit the action")
	}
	collector, asset, err := shared.ResolveConnectorActionCollector(d.AssetStore, d.HubCollectorStore, "portainer", req)
	if err != nil {
		return connectorsdk.ActionResult{}, err
	}
	if (asset == nil && savedAsset != "") || (asset != nil && (asset.ID != savedAsset || req.TargetID != savedAsset)) {
		return connectorsdk.ActionResult{}, fmt.Errorf("Portainer action's stored target disappeared or changed")
	}
	requested, err := requestedPortainerIdentity(key, req, asset)
	if err != nil {
		return connectorsdk.ActionResult{}, err
	}
	identity := portainerActionIdentity{key: key, resource: strings.TrimSpace(req.Params[key])}
	identity.endpoint, err = positivePortainerID(req.Params["endpoint_id"])
	if err != nil {
		return connectorsdk.ActionResult{}, err
	}
	if key == "container_id" {
		if !fullPortainerContainerID(identity.resource) {
			return connectorsdk.ActionResult{}, fmt.Errorf("saved Portainer container identity is invalid")
		}
		if (asset != nil || fullPortainerContainerID(requested.resource)) && requested.resource != identity.resource {
			return connectorsdk.ActionResult{}, fmt.Errorf("saved Portainer container no longer matches the target")
		}
	} else if requested.resource != identity.resource {
		return connectorsdk.ActionResult{}, fmt.Errorf("saved Portainer stack no longer matches the target")
	}
	runtime, err := d.LoadPortainerRuntime(collector.ID)
	if err != nil {
		return connectorsdk.ActionResult{}, err
	}
	if runtime.CollectorID != collector.ID {
		return connectorsdk.ActionResult{}, fmt.Errorf("Portainer runtime owner mismatch")
	}
	if key == "stack_id" {
		if err := verifyPortainerStack(ctx, runtime, identity); err != nil {
			return connectorsdk.ActionResult{}, err
		}
	}
	if err := d.checkPortainerActionOwners(ctx, asset, collector.ID, identity); err != nil {
		return connectorsdk.ActionResult{}, err
	}
	// This provider-local target is distinct from the retained logical audit
	// target. The shared worker result keeps the original action/run identity.
	invoke := req
	invoke.Params = maps.Clone(req.Params)
	if key == "container_id" {
		invoke.TargetID = "portainer-container-" + strconv.Itoa(identity.endpoint) + "-" + identity.resource
	} else {
		invoke.TargetID = "portainer-stack-" + identity.resource
	}
	connector := portainerconnector.NewWithClient(runtime.Client)
	actionID = modelmap.ResolveActionID(actionID, req.TargetID, connector.Actions())
	result, err := connector.ExecuteAction(ctx, actionID, invoke)
	if result.Metadata == nil {
		result.Metadata = make(map[string]string)
	}
	result.Metadata["collector_id"] = collector.ID
	result.Metadata["endpoint_id"] = strconv.Itoa(identity.endpoint)
	result.Metadata[key] = identity.resource
	return result, err
}

func (d *Deps) checkPortainerActionOwners(ctx context.Context, asset *assets.Asset, collectorID string, identity portainerActionIdentity) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	targets, err := d.portainerMutationOwners(asset, collectorID, identity.endpoint, identity.key, identity.resource)
	if err != nil {
		return err
	}
	if err := maintenanceguard.CheckAssetActions(targets, d.EvaluateAssetGuardrails); err != nil {
		return err
	}
	return ctx.Err()
}

package portainer

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"strconv"
	"strings"

	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/assets"
	portainerconnector "github.com/labtether/labtether/internal/connectors/portainer"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/modelmap"
)

const portainerBoundAsset = "portainer_asset_id"

type portainerActionIdentity struct {
	endpoint      int
	key, resource string
}

// BindAction resolves mutable names before enqueueing. The logical target is
// retained for audit; only the copied params carry the actual dispatch identity.
func (d *Deps) BindAction(ctx context.Context, actionID string, req connectorsdk.ActionRequest) (connectorsdk.ActionRequest, error) {
	if err := ctx.Err(); err != nil {
		return req, err
	}
	key, err := portainerActionKey(actionID, req.TargetID)
	if err != nil {
		return req, err
	}
	collector, asset, err := shared.ResolveConnectorActionCollector(d.AssetStore, d.HubCollectorStore, "portainer", req)
	if err != nil {
		return req, err
	}
	runtime, err := d.LoadPortainerRuntime(collector.ID)
	if err != nil {
		return req, err
	}
	if runtime.CollectorID != collector.ID {
		return req, fmt.Errorf("Portainer runtime owner mismatch")
	}
	identity, err := requestedPortainerIdentity(key, req, asset)
	if err != nil {
		return req, err
	}
	if key == "container_id" {
		data, err := runtime.Client.InspectContainer(ctx, identity.endpoint, identity.resource)
		if err != nil {
			return req, fmt.Errorf("cannot inspect Portainer container: %w", err)
		}
		var inspected struct {
			ID string `json:"Id"`
		}
		if err := json.Unmarshal(data, &inspected); err != nil || !fullPortainerContainerID(inspected.ID) {
			return req, fmt.Errorf("Portainer returned an invalid full container identity")
		}
		if asset != nil && inspected.ID != identity.resource {
			return req, fmt.Errorf("Portainer container differs from the stored asset")
		}
		if fullPortainerContainerID(identity.resource) && inspected.ID != identity.resource {
			return req, fmt.Errorf("Portainer inspect returned a different container")
		}
		identity.resource = inspected.ID
	} else if err := verifyPortainerStack(ctx, runtime, identity); err != nil {
		return req, err
	}
	if override := strings.TrimSpace(req.Params[key]); override != "" && override != identity.resource {
		return req, fmt.Errorf("Portainer resource override differs from the selected target")
	}
	req.Params = maps.Clone(req.Params)
	if req.Params == nil {
		req.Params = make(map[string]string)
	}
	// Presence records that binding ran, even for a raw target with no row.
	// Never accept a caller's claim that a target was (or was not) an asset.
	req.Params[portainerBoundAsset] = ""
	delete(req.Params, "container_id")
	delete(req.Params, "stack_id")
	if asset != nil {
		req.Params[portainerBoundAsset] = asset.ID
	}
	req.Params["collector_id"] = collector.ID
	req.Params["endpoint_id"] = strconv.Itoa(identity.endpoint)
	req.Params[key] = identity.resource
	if err := d.checkPortainerActionOwners(ctx, asset, collector.ID, identity); err != nil {
		return req, err
	}
	return req, nil
}

func portainerActionKey(actionID, target string) (string, error) {
	actionID = modelmap.ResolveActionID(actionID, target, (&portainerconnector.Connector{}).Actions())
	switch actionID {
	case "container.start", "container.stop", "container.restart", "container.kill", "container.pause", "container.unpause", "container.remove":
		return "container_id", nil
	case "stack.start", "stack.stop", "stack.remove", "stack.redeploy":
		return "stack_id", nil
	default:
		return "", fmt.Errorf("unsupported Portainer action")
	}
}

func positivePortainerID(value string) (int, error) {
	id, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("Portainer identity must be a positive integer")
	}
	return id, nil
}

func requestedPortainerIdentity(key string, req connectorsdk.ActionRequest, asset *assets.Asset) (portainerActionIdentity, error) {
	identity := portainerActionIdentity{key: key}
	var err error
	if asset != nil {
		kind := "container"
		if key == "stack_id" {
			kind = "stack"
		}
		if asset.Type != kind {
			return identity, fmt.Errorf("Portainer target is not a %s", kind)
		}
		identity.endpoint, err = positivePortainerID(asset.Metadata["endpoint_id"])
		identity.resource = strings.TrimSpace(asset.Metadata[key])
		if key == "container_id" && !fullPortainerContainerID(identity.resource) {
			return identity, fmt.Errorf("stored Portainer container has no full identity")
		}
	} else {
		native := assetid.NativeCollectorAssetID(strings.TrimSpace(req.TargetID))
		if key == "container_id" {
			if !strings.HasPrefix(native, "portainer-container-") {
				return identity, fmt.Errorf("invalid Portainer container target")
			}
			ep, resource, ok := strings.Cut(strings.TrimPrefix(native, "portainer-container-"), "-")
			if !ok || strings.TrimSpace(resource) == "" {
				return identity, fmt.Errorf("invalid Portainer container target")
			}
			identity.endpoint, err = positivePortainerID(ep)
			identity.resource = resource
		} else {
			if !strings.HasPrefix(native, "portainer-stack-") {
				return identity, fmt.Errorf("invalid Portainer stack target")
			}
			identity.resource = strings.TrimPrefix(native, "portainer-stack-")
			identity.endpoint, err = positivePortainerID(req.Params["endpoint_id"])
		}
	}
	if err != nil {
		return identity, err
	}
	if key == "stack_id" {
		id, err := positivePortainerID(identity.resource)
		if err != nil {
			return identity, err
		}
		identity.resource = strconv.Itoa(id)
	}
	if asset != nil {
		// Discovery derives the native ID from these same provider fields.
		// A new request must not follow metadata drift into another resource.
		expected := "portainer-stack-" + identity.resource
		if key == "container_id" {
			expected = "portainer-container-" + strconv.Itoa(identity.endpoint) + "-" + identity.resource[:12]
		}
		if assetid.NativeCollectorAssetID(asset.ID) != expected {
			return identity, fmt.Errorf("stored Portainer asset ID conflicts with its resource metadata")
		}
	}
	if override := strings.TrimSpace(req.Params["endpoint_id"]); override != "" {
		ep, err := positivePortainerID(override)
		if err != nil || ep != identity.endpoint {
			return identity, fmt.Errorf("Portainer endpoint override differs from the target")
		}
	}
	return identity, nil
}

func verifyPortainerStack(ctx context.Context, runtime *PortainerRuntime, identity portainerActionIdentity) error {
	stacks, err := runtime.Client.GetStacks(ctx)
	if err != nil {
		return fmt.Errorf("cannot verify Portainer stack endpoint: %w", err)
	}
	for _, stack := range stacks {
		if strconv.Itoa(stack.ID) == identity.resource && stack.EndpointID == identity.endpoint {
			return nil
		}
	}
	return fmt.Errorf("Portainer stack is not in the selected endpoint")
}

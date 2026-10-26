package actionspkg

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/modelmap"
	"github.com/labtether/labtether/internal/servicehttp"
)

// Connector targets can be provider references, and some actions ignore their
// target entirely. Restricted callers must instead name a stored asset and an
// operation whose effective provider target cannot be changed by its params.
func (d *Deps) authorizeConnectorAction(w http.ResponseWriter, r *http.Request, req *actions.ExecuteRequest) bool {
	allowed := apiv2.AllowedAssetsFromContext(r.Context())
	restricted := len(allowed) > 0
	actionID := req.ActionID
	if d.ConnectorRegistry != nil {
		if connector, ok := d.ConnectorRegistry.Get(req.ConnectorID); ok {
			actionID = modelmap.ResolveActionID(actionID, req.Target, connector.Actions())
		}
	}
	trueNASVM := req.ConnectorID == "truenas" && (actionID == "vm.start" || actionID == "vm.stop")
	if !restricted && (!trueNASVM || req.Params["vm_id"] != "" || req.Target == "") {
		return true
	}
	deny := func() bool {
		servicehttp.WriteError(w, http.StatusForbidden, "connector action is not authorized for this asset")
		return false
	}
	if restricted && (req.Target == "" || !apiv2.AssetCheck(allowed, req.Target)) {
		return deny()
	}
	if d.AssetStore == nil {
		if !restricted {
			return true
		}
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "asset store unavailable")
		return false
	}
	asset, ok, err := d.AssetStore.GetAsset(req.Target)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to resolve action target")
		return false
	}
	if !ok || !strings.EqualFold(strings.TrimSpace(asset.Source), req.ConnectorID) {
		if restricted {
			return deny()
		}
		return true
	}
	if restricted {
		params, supported := singleAssetActionParams(req.ConnectorID, actionID, asset)
		if !supported {
			return deny()
		}
		for key := range req.Params {
			if key != "group_id" && key != "requested_actor_label" && !params[key] {
				return deny()
			}
		}
		if override := req.Params["collector_id"]; override != "" && override != strings.TrimSpace(asset.Metadata["collector_id"]) {
			return deny()
		}
		if req.ConnectorID == "portainer" {
			if override := req.Params["endpoint_id"]; override != "" && override != strings.TrimSpace(asset.Metadata["endpoint_id"]) {
				return deny()
			}
		}
	}
	if trueNASVM && strings.EqualFold(asset.Type, "vm") {
		normalized, valid := modelmap.TrueNASVMActionParams(actionID, asset, req.Params)
		if !valid {
			if restricted {
				return deny()
			}
			return true // Preserve legacy numeric provider targets without metadata.
		}
		if override := req.Params["vm_id"]; restricted && override != "" {
			requestedID, parseErr := strconv.Atoi(override)
			if parseErr != nil || strconv.Itoa(requestedID) != normalized["vm_id"] {
				return deny()
			}
		}
		// Configured binders own unrestricted target resolution. Do not turn an
		// earlier metadata read into an apparent explicit VM override.
		configured := d.ConfiguredConnectorActions[req.ConnectorID]
		if restricted || configured.Bind == nil {
			req.Params = normalized
		}
	}
	if restricted {
		// Use the same resolved operation that was authorized, including aliases.
		req.ActionID = actionID
	}
	return true
}

// These are the single-asset operations implemented by the current providers.
// New actions/params require an explicit review of their effective target.
func singleAssetActionParams(connectorID, actionID string, asset assets.Asset) (map[string]bool, bool) {
	params := make(map[string]bool)
	nativeID := assetid.NativeCollectorAssetID(asset.ID)
	switch connectorID {
	case "proxmox":
		kind := strings.ToLower(strings.TrimSpace(asset.Metadata["proxmox_type"]))
		prefix := ""
		switch {
		case strings.EqualFold(asset.Type, "vm") && (kind == "" || kind == "qemu"):
			prefix = "vm."
		case strings.EqualFold(asset.Type, "container") && (kind == "" || kind == "lxc"):
			prefix = "ct."
		default:
			return nil, false
		}
		if !strings.HasPrefix(actionID, prefix) || strings.Contains(asset.ID, "/") {
			return nil, false
		}
		vmID := strings.TrimSpace(asset.Metadata["vmid"])
		if vmID == "" {
			vmID = strings.TrimPrefix(nativeID, "proxmox-"+strings.TrimSuffix(prefix, ".")+"-")
		}
		if id, err := strconv.Atoi(vmID); err != nil || id <= 0 {
			return nil, false
		}
		if strings.TrimSpace(asset.Metadata["node"]) == "" && strings.TrimSpace(asset.Name) == "" {
			return nil, false
		}
		switch strings.TrimPrefix(actionID, prefix) {
		case "start", "stop", "shutdown", "reboot", "force_stop":
		case "suspend", "resume":
			if prefix != "vm." {
				return nil, false
			}
		case "snapshot", "snapshot.delete", "snapshot.rollback":
			params["snapshot_name"] = true
		case "disk_resize":
			if prefix != "vm." {
				return nil, false
			}
			params["disk"], params["size"] = true, true
		default:
			return nil, false
		}
		params["collector_id"] = true
	case "docker", "portainer":
		if connectorID == "docker" {
			if asset.Type != "docker-container" || !strings.HasPrefix(asset.ID, "docker-ct-") {
				return nil, false
			}
		} else {
			params["collector_id"], params["endpoint_id"] = true, true
			if asset.Type == "stack" {
				if !strings.HasPrefix(nativeID, "portainer-stack-") {
					return nil, false
				}
				id, err := strconv.Atoi(strings.TrimPrefix(nativeID, "portainer-stack-"))
				if err != nil || id <= 0 || strconv.Itoa(id) != strings.TrimSpace(asset.Metadata["stack_id"]) {
					return nil, false
				}
				if endpoint, err := strconv.Atoi(asset.Metadata["endpoint_id"]); err != nil || endpoint <= 0 {
					return nil, false
				}
				switch actionID {
				case "stack.start", "stack.stop", "stack.remove":
				case "stack.redeploy":
					params["pull_image"] = true
				default:
					return nil, false
				}
				return params, true
			}
			if asset.Type != "container" || !strings.HasPrefix(nativeID, "portainer-container-") {
				return nil, false
			}
			endpoint, container, found := strings.Cut(strings.TrimPrefix(nativeID, "portainer-container-"), "-")
			if id, err := strconv.Atoi(endpoint); err != nil || id <= 0 || !found || container == "" {
				return nil, false
			}
		}
		switch actionID {
		case "container.start", "container.pause", "container.unpause":
		case "container.stop", "container.restart":
			if connectorID == "docker" {
				params["timeout"] = true
			}
		case "container.kill":
			if connectorID == "docker" {
				params["signal"] = true
			}
		case "container.remove":
			params["force"] = true
		default:
			return nil, false
		}
	case "truenas":
		if asset.Type != "vm" || (actionID != "vm.start" && actionID != "vm.stop") {
			return nil, false
		}
		params["vm_id"] = true
	default:
		return nil, false
	}
	return params, true
}

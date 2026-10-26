package maintenanceguard

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/assets"
	proxmoxconnector "github.com/labtether/labtether/internal/connectors/proxmox"
	tnconnector "github.com/labtether/labtether/internal/connectors/truenas"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/persistence"
)

func (e ConnectorEvaluator) proxmoxTargets(actionID string, req connectorsdk.ActionRequest) ([]string, error) {
	if e.ResolveProxmoxDestination == nil {
		return nil, fmt.Errorf("Proxmox destination resolver unavailable")
	}
	node, vmID, collectorID, err := e.ResolveProxmoxDestination(actionID, req.TargetID, req.Params)
	if err != nil {
		return nil, err
	}
	kind := "vm"
	if strings.HasPrefix(actionID, "ct.") {
		kind = "ct"
	}
	var targets []string
	nativeIDs := []string{"proxmox-" + kind + "-" + vmID, proxmoxconnector.NodeAssetID(node)}
	if destination := proxmoxconnector.ActionTargetNode(actionID, req.Params); destination != "" {
		nativeIDs = append(nativeIDs, proxmoxconnector.NodeAssetID(destination))
	}
	for _, native := range nativeIDs {
		ids, err := e.nativeAssetTargets("proxmox", native, collectorID)
		if err != nil {
			return nil, err
		}
		targets = append(targets, ids...)
	}
	parent, err := e.collectorParent(collectorID, "proxmox")
	return append(targets, parent), err
}

func (e ConnectorEvaluator) nativeAssetTargets(source, native, collectorID string) ([]string, error) {
	ids := []string{native}
	if collectorID != "" {
		ids = append(ids, assetid.ScopeCollectorAssetID(native, collectorID))
	}
	var targets []string
	for _, id := range ids {
		asset, found, err := e.storedAsset(id)
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		if id == native {
			// Discovery deliberately preserves an old native row owned by a
			// different collector. It is not an owner of this dispatch target.
			if owner := strings.TrimSpace(asset.Metadata["collector_id"]); owner != "" && owner != collectorID {
				continue
			}
		}
		if asset.Source != source {
			return nil, fmt.Errorf("provider asset has a different source")
		}
		if err := verifyCollectorAsset(asset, collectorID); err != nil {
			return nil, err
		}
		targets = append(targets, asset.ID)
	}
	return targets, nil
}

func verifyCollectorAsset(asset assets.Asset, collectorID string) error {
	owner := strings.TrimSpace(asset.Metadata["collector_id"])
	if scope, scoped := assetid.CollectorScopeFromAssetID(asset.ID); scoped {
		if collectorID == "" || scope != assetid.CollectorScope(collectorID) || (owner != "" && owner != collectorID) {
			return fmt.Errorf("provider asset collector does not match dispatch")
		}
		return nil
	}
	if owner != collectorID {
		return fmt.Errorf("legacy provider asset collector is ambiguous")
	}
	return nil
}

func (e ConnectorEvaluator) collectorParent(id, kind string) (string, error) {
	if id == "" {
		return "", nil
	}
	if e.Collectors == nil {
		return "", fmt.Errorf("collector inventory unavailable")
	}
	collector, found, err := e.Collectors.GetHubCollector(id)
	if err != nil {
		return "", err
	}
	if !found || !collector.Enabled || hubcollector.NormalizeCollectorType(collector.CollectorType) != kind {
		return "", fmt.Errorf("action collector is inactive or mismatched")
	}
	return strings.TrimSpace(collector.AssetID), nil
}

func (e ConnectorEvaluator) trueNASTargets(actionID string, req connectorsdk.ActionRequest) ([]string, error) {
	asset, found, err := e.storedAsset(req.TargetID)
	if err != nil {
		return nil, err
	}
	collectorID, err := e.ResolveTrueNASActionCollector(req)
	if err != nil {
		return nil, err
	}
	if found && asset.Source == "truenas" {
		if err := verifyCollectorAsset(asset, collectorID); err != nil {
			return nil, err
		}
	}
	filters := []struct{ kind, vmID string }{{"nas", ""}}
	if actionID == "vm.start" || actionID == "vm.stop" || actionID == "vm.restart" {
		vmID := strings.TrimSpace(req.Params["vm_id"])
		if vmID == "" && found && asset.Source == "truenas" && asset.Type == "vm" {
			vmID = strings.TrimSpace(asset.Metadata["vm_id"])
		}
		if vmID == "" {
			vmID = strings.TrimPrefix(assetid.NativeCollectorAssetID(req.TargetID), "truenas-vm-")
		}
		parsed, err := strconv.Atoi(vmID)
		if err != nil || parsed <= 0 {
			return nil, fmt.Errorf("cannot resolve TrueNAS VM ID")
		}
		filters = append(filters, struct{ kind, vmID string }{"vm", strconv.Itoa(parsed)})
	}
	lookup, ok := e.Assets.(persistence.TrueNASActionAssetStore)
	if !ok {
		return nil, fmt.Errorf("TrueNAS provider identity lookup unavailable")
	}
	var targets []string
	for _, filter := range filters {
		matches, err := lookup.FindTrueNASActionAssets(collectorID, filter.kind, filter.vmID)
		if err != nil {
			return nil, err
		}
		if len(matches) > 1 {
			return nil, fmt.Errorf("TrueNAS %s inventory is ambiguous", filter.kind)
		}
		for _, match := range matches {
			if err := verifyCollectorAsset(match, collectorID); err != nil {
				return nil, err
			}
			targets = append(targets, match.ID)
		}
	}
	nativeIDs, err := tnconnector.ActionResourceAssetIDs(actionID, req)
	if err != nil {
		return nil, err
	}
	for _, native := range nativeIDs {
		ids, err := e.nativeAssetTargets("truenas", native, collectorID)
		if err != nil {
			return nil, err
		}
		targets = append(targets, ids...)
	}
	parent, err := e.collectorParent(collectorID, "truenas")
	return append(targets, parent), err
}

// ResolveTrueNASActionCollector preserves a queued collector binding. A missing
// or disabled owner is an error, never a reason to choose a different NAS.
func (e ConnectorEvaluator) ResolveTrueNASActionCollector(req connectorsdk.ActionRequest) (string, error) {
	if e.Collectors != nil {
		selected, _, err := shared.ResolveConnectorActionCollector(e.Assets, e.Collectors, "truenas", req)
		if err != nil {
			return "", err
		}
		return selected.ID, nil
	}
	// Low-level legacy catalog fixtures have no collector store. This cannot
	// supply or validate a configured runtime, nor accept a saved collector.
	if strings.TrimSpace(req.Params["collector_id"]) != "" {
		return "", fmt.Errorf("collector inventory unavailable")
	}
	if _, scoped := assetid.CollectorScopeFromAssetID(req.TargetID); scoped {
		return "", fmt.Errorf("cannot verify scoped asset without collector inventory")
	}
	if strings.TrimSpace(req.TargetID) != "" {
		asset, found, err := e.storedAsset(req.TargetID)
		if err != nil {
			return "", err
		}
		if found {
			if asset.Source != "truenas" {
				return "", fmt.Errorf("action target is not a TrueNAS asset")
			}
			if err := verifyCollectorAsset(asset, ""); err != nil {
				return "", err
			}
		}
	}
	return "", nil
}

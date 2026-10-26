package main

import (
	"context"
	"fmt"
	"maps"
	"strconv"
	"strings"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/assets"
	tnconnector "github.com/labtether/labtether/internal/connectors/truenas"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/hubapi/maintenanceguard"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/modelmap"
)

const trueNASTargetVMParam = "_truenas_target_vm_id"

// Capture the actual provider VM before queueing. The original audit target
// stays unchanged, including when an unrestricted caller requests an override.
func (s *apiServer) bindTrueNASAction(ctx context.Context, actionID string, req connectorsdk.ActionRequest) (connectorsdk.ActionRequest, error) {
	actionID, err := resolveTrueNASBoundAction(actionID, req.TargetID)
	if err != nil {
		return req, err
	}
	if err := ctx.Err(); err != nil {
		return req, err
	}
	req.Params = maps.Clone(req.Params)
	collector, asset, err := shared.ResolveConnectorActionCollector(s.assetStore, s.hubCollectorStore, "truenas", req)
	if err != nil {
		return req, err
	}
	if err := ctx.Err(); err != nil {
		return req, err
	}
	if err := validateTrueNASBoundTarget(actionID, req.TargetID, asset); err != nil {
		return req, err
	}
	if req.Params == nil {
		req.Params = make(map[string]string)
	}
	delete(req.Params, trueNASTargetVMParam)
	if asset != nil {
		req.Params[trueNASTargetVMParam] = asset.Metadata["vm_id"]
	}
	if actionID == "vm.start" || actionID == "vm.stop" {
		vmID := strings.TrimSpace(req.Params["vm_id"])
		if vmID == "" {
			if asset != nil && asset.Type == "vm" {
				vmID = asset.Metadata["vm_id"]
			} else if asset == nil {
				vmID = strings.TrimSpace(req.TargetID)
			}
		}
		id, err := strconv.Atoi(vmID)
		if err != nil || id <= 0 {
			return req, fmt.Errorf("exact TrueNAS VM ID is required")
		}
		// Authorization may have read an earlier inventory snapshot. Restricted
		// callers cannot use its synthesized VM ID after this asset changed.
		if len(apiv2.AllowedAssetsFromContext(ctx)) > 0 {
			if asset == nil || asset.Type != "vm" {
				return req, fmt.Errorf("restricted TrueNAS action requires a current VM asset")
			}
			currentID, err := strconv.Atoi(strings.TrimSpace(asset.Metadata["vm_id"]))
			if err != nil || currentID <= 0 || id != currentID {
				return req, fmt.Errorf("TrueNAS VM identity changed after authorization")
			}
		}
		req.Params["vm_id"] = strconv.Itoa(id)
	}
	req.Params["collector_id"] = collector.ID
	return req, nil
}

// Execute consumes saved routing values; a deleted or changed asset never
// refills the request with another VM or selects a replacement NAS.
func (s *apiServer) executeTrueNASActionDirect(ctx context.Context, actionID string, req connectorsdk.ActionRequest) (connectorsdk.ActionResult, error) {
	actionID, err := resolveTrueNASBoundAction(actionID, req.TargetID)
	if err != nil {
		return connectorsdk.ActionResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return connectorsdk.ActionResult{}, err
	}
	req.Params = maps.Clone(req.Params)
	id := req.Params["collector_id"]
	if id == "" || id != strings.TrimSpace(id) {
		return connectorsdk.ActionResult{}, fmt.Errorf("TrueNAS action has no saved collector binding; resubmit the action")
	}
	if actionID == "vm.start" || actionID == "vm.stop" {
		vmID, err := strconv.Atoi(req.Params["vm_id"])
		if err != nil || vmID <= 0 || strconv.Itoa(vmID) != req.Params["vm_id"] {
			return connectorsdk.ActionResult{}, fmt.Errorf("TrueNAS action has no saved VM binding; resubmit the action")
		}
	}
	collector, asset, err := shared.ResolveConnectorActionCollector(s.assetStore, s.hubCollectorStore, "truenas", req)
	if err != nil {
		return connectorsdk.ActionResult{}, err
	}
	if collector.ID != id {
		return connectorsdk.ActionResult{}, fmt.Errorf("TrueNAS action collector changed; resubmit the action")
	}
	if err := validateTrueNASBoundTarget(actionID, req.TargetID, asset); err != nil {
		return connectorsdk.ActionResult{}, err
	}
	oldVM, canonical := req.Params[trueNASTargetVMParam]
	if (asset != nil) != canonical || (asset != nil && asset.Metadata["vm_id"] != oldVM) {
		return connectorsdk.ActionResult{}, fmt.Errorf("TrueNAS action target changed; resubmit the action")
	}
	if err := ctx.Err(); err != nil {
		return connectorsdk.ActionResult{}, err
	}
	runtime, err := s.ensureTruenasDeps().LoadTrueNASRuntime(id)
	if err != nil {
		return connectorsdk.ActionResult{}, err
	}
	if runtime.CollectorID != id {
		return connectorsdk.ActionResult{}, fmt.Errorf("TrueNAS runtime collector mismatch")
	}
	connector := tnconnector.NewWithConfig(tnconnector.Config{BaseURL: runtime.BaseURL, APIKey: runtime.APIKey, SkipVerify: runtime.SkipVerify, Timeout: runtime.Timeout})
	if err := ctx.Err(); err != nil {
		return connectorsdk.ActionResult{}, err
	}
	if err := maintenanceguard.CheckConnectorExecution("truenas", actionID, req, s.connectorMaintenanceEvaluator()); err != nil {
		return connectorsdk.ActionResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return connectorsdk.ActionResult{}, err
	}
	return connector.ExecuteAction(ctx, actionID, req)
}

func resolveTrueNASBoundAction(actionID, target string) (string, error) {
	catalog := (&tnconnector.Connector{}).Actions()
	actionID = modelmap.ResolveActionID(actionID, target, catalog)
	for _, action := range catalog {
		if action.ID == actionID {
			return actionID, nil
		}
	}
	return "", fmt.Errorf("unsupported TrueNAS action")
}

func validateTrueNASBoundTarget(actionID, target string, asset *assets.Asset) error {
	if asset != nil {
		return nil // The shared resolver verified the provider and collector.
	}
	_, scoped := assetid.CollectorScopeFromAssetID(target)
	missingVM := (actionID == "vm.start" || actionID == "vm.stop") && strings.HasPrefix(strings.TrimSpace(target), "truenas-vm-")
	if scoped || missingVM {
		return fmt.Errorf("TrueNAS asset is missing; resubmit with its exact provider resource")
	}
	return nil
}

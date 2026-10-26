package homeassistantpkg

import (
	"context"
	"fmt"
	"maps"
	"reflect"
	"strings"

	"github.com/labtether/labtether/internal/connectorsdk"
)

// ExecuteAction never fills missing routing values from a replacement provider.
// Credentials are loaded from the exact saved collector, never the env catalog.
func (d *Deps) ExecuteAction(ctx context.Context, actionID string, req connectorsdk.ActionRequest) (connectorsdk.ActionResult, error) {
	fail := func(err error) (connectorsdk.ActionResult, error) { return connectorsdk.ActionResult{}, err }
	req.Params = maps.Clone(req.Params)
	actionID, err := resolveHAAction(actionID, req)
	if err != nil {
		return fail(err)
	}
	collectorID, entity := req.Params["collector_id"], req.Params[haEntityParam]
	_, hasEntity := req.Params[haEntityParam]
	if req.Params[haBindingParam] != "1" || collectorID == "" || collectorID != strings.TrimSpace(collectorID) || !hasEntity {
		return fail(fmt.Errorf("Home Assistant action has no saved binding; resubmit the action"))
	}
	collector, asset, currentEntity, err := d.resolveHAActionOwner(ctx, req)
	if err != nil {
		return fail(err)
	}
	canonicalEntity, canonical := req.Params[haTargetEntityParam]
	if collector.ID != collectorID || currentEntity != entity || (asset != nil) != canonical ||
		(asset != nil && (currentEntity != canonicalEntity || strings.TrimSpace(asset.Metadata["domain"]) != req.Params[haTargetDomainParam])) ||
		(actionID == "entity.toggle" && entity == "") {
		return fail(fmt.Errorf("Home Assistant action target changed; resubmit the action"))
	}
	capturedConfig := maps.Clone(collector.Config)
	runtime, err := d.loadRuntimeForCollector(collector)
	if err != nil {
		return fail(err)
	}
	if err := d.checkHAActionMaintenance(ctx, actionID, req.TargetID, collector, entity); err != nil {
		return fail(err)
	}
	// Recheck identity after potentially slow maintenance reads. This does not
	// claim a transaction with HA; cancellation cannot undo an accepted service.
	latest, latestAsset, latestEntity, err := d.resolveHAActionOwner(ctx, req)
	if err != nil {
		return fail(err)
	}
	if collector.ID != latest.ID || collector.AssetID != latest.AssetID || !reflect.DeepEqual(capturedConfig, latest.Config) ||
		latestEntity != entity || (latestAsset != nil) != canonical ||
		(latestAsset != nil && strings.TrimSpace(latestAsset.Metadata["domain"]) != req.Params[haTargetDomainParam]) {
		return fail(fmt.Errorf("Home Assistant action owner changed before dispatch"))
	}
	params := make(map[string]string, len(req.Params))
	for key, value := range req.Params {
		if key == "collector_id" || key == "group_id" || key == "requested_actor_label" || strings.HasPrefix(key, "_ha_") {
			continue
		}
		params[key] = value
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	return runtime.Connector.ExecuteAction(ctx, actionID, connectorsdk.ActionRequest{TargetID: entity, Params: params, DryRun: req.DryRun})
}

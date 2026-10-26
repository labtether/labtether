package pbs

import (
	"context"
	"fmt"
	"maps"
	"strings"

	pbsconnector "github.com/labtether/labtether/internal/connectors/pbs"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/hubapi/maintenanceguard"
	"github.com/labtether/labtether/internal/hubapi/shared"
)

// ExecuteAction uses only saved routing values and rechecks current ownership
// and maintenance. Old unbound jobs cannot follow a replacement singleton.
func (d *Deps) ExecuteAction(ctx context.Context, actionID string, req connectorsdk.ActionRequest) (connectorsdk.ActionResult, error) {
	fail := func(err error) (connectorsdk.ActionResult, error) { return connectorsdk.ActionResult{}, err }
	actionID, err := resolvePBSAction(actionID, req.TargetID)
	if err != nil {
		return fail(err)
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	req.Params = maps.Clone(req.Params)
	collectorID, store := req.Params["collector_id"], req.Params["store"]
	if strings.TrimSpace(collectorID) == "" || collectorID != strings.TrimSpace(collectorID) || !pbsDatastoreName.MatchString(store) {
		return fail(fmt.Errorf("PBS action has no valid saved collector/datastore binding; resubmit the action"))
	}
	collector, asset, err := shared.ResolveConnectorActionCollector(d.AssetStore, d.HubCollectorStore, "pbs", req)
	if err != nil {
		return fail(err)
	}
	if collector.ID != collectorID {
		return fail(fmt.Errorf("PBS action collector changed; resubmit the action"))
	}
	if err := validatePBSActionAsset(req.TargetID, asset); err != nil {
		return fail(err)
	}
	oldStore, canonical := req.Params[pbsTargetStoreParam]
	if (asset != nil) != canonical || (asset != nil && asset.Metadata["store"] != oldStore) {
		return fail(fmt.Errorf("PBS action target changed; resubmit the action"))
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	runtime, err := d.LoadPBSRuntime(collectorID)
	if err != nil {
		return fail(err)
	}
	if runtime.CollectorID != collectorID || runtime.Client == nil {
		return fail(fmt.Errorf("PBS action runtime does not match its saved collector"))
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	if d.EvaluateConnectorGuardrails == nil {
		return fail(fmt.Errorf("PBS action maintenance evaluator unavailable"))
	}
	if err := maintenanceguard.CheckConnectorExecution("pbs", actionID, req, d.EvaluateConnectorGuardrails); err != nil {
		return fail(err)
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	return pbsconnector.NewWithClient(runtime.Client).ExecuteAction(ctx, actionID, req)
}

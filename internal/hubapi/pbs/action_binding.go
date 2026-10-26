package pbs

import (
	"context"
	"fmt"
	"maps"
	"strings"

	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/assets"
	pbsconnector "github.com/labtether/labtether/internal/connectors/pbs"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/modelmap"
)

const pbsTargetStoreParam = "_pbs_target_store"

// BindAction records provider identities before queueing. TargetID remains the
// submitted audit target; normalized asset IDs never become datastore names.
func (d *Deps) BindAction(ctx context.Context, actionID string, req connectorsdk.ActionRequest) (connectorsdk.ActionRequest, error) {
	if _, err := resolvePBSAction(actionID, req.TargetID); err != nil {
		return req, err
	}
	if err := ctx.Err(); err != nil {
		return req, err
	}
	req.Params = maps.Clone(req.Params)
	collector, asset, err := shared.ResolveConnectorActionCollector(d.AssetStore, d.HubCollectorStore, "pbs", req)
	if err != nil {
		return req, err
	}
	if err := ctx.Err(); err != nil {
		return req, err
	}
	if err := validatePBSActionAsset(req.TargetID, asset); err != nil {
		return req, err
	}
	params := req.Params
	if params == nil {
		params = make(map[string]string)
	}
	delete(params, pbsTargetStoreParam)
	store := strings.TrimSpace(params["store"])
	if asset != nil {
		// Even an empty server metadata value is a present ownership marker.
		params[pbsTargetStoreParam] = asset.Metadata["store"]
		if store == "" {
			store = asset.Metadata["store"]
		}
	} else if store == "" {
		store = strings.TrimSpace(req.TargetID)
	}
	if !pbsDatastoreName.MatchString(store) {
		return req, fmt.Errorf("exact PBS datastore name is required")
	}
	params["collector_id"], params["store"] = collector.ID, store
	req.Params = params
	return req, nil
}

func resolvePBSAction(actionID, target string) (string, error) {
	actionID = modelmap.ResolveActionID(actionID, target, pbsconnector.NewWithClient(nil).Actions())
	switch actionID {
	case "datastore.verify", "datastore.prune", "datastore.gc":
		return actionID, nil
	default:
		return "", fmt.Errorf("unsupported PBS action")
	}
}

func validatePBSActionAsset(target string, asset *assets.Asset) error {
	if asset != nil {
		if asset.Source != "pbs" || (asset.Type != "storage-pool" && asset.Type != "storage-controller") {
			return fmt.Errorf("PBS action target is not a datastore or server")
		}
		return nil
	}
	nativeID := assetid.NativeCollectorAssetID(target)
	_, scoped := assetid.CollectorScopeFromAssetID(target)
	if scoped || strings.HasPrefix(nativeID, "pbs-datastore-") || strings.HasPrefix(nativeID, "pbs-server-") {
		return fmt.Errorf("PBS asset is missing; resubmit with its exact datastore and collector")
	}
	return nil
}

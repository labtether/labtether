package maintenanceguard

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/persistence"
)

func (e ConnectorEvaluator) pbsTargets(action string, req connectorsdk.ActionRequest) ([]string, error) {
	collector := strings.TrimSpace(req.Params["collector_id"])
	if collector == "" {
		return nil, fmt.Errorf("PBS mutation requires a resolved collector")
	}
	parent, err := e.collectorParent(collector, "pbs")
	if err != nil {
		return nil, err
	}
	lookup, ok := e.Assets.(persistence.PBSActionAssetStore)
	if !ok {
		return nil, fmt.Errorf("PBS identity lookup unavailable")
	}
	var stores []string
	if raw := req.Params["_pbs_stores"]; raw != "" {
		if len(raw) > 4096 || json.Unmarshal([]byte(raw), &stores) != nil || len(stores) > 8 {
			return nil, fmt.Errorf("invalid PBS mutation datastore identities")
		}
	}
	store := strings.TrimSpace(req.Params["store"])
	if store == "" && strings.HasPrefix(action, "datastore.") {
		// Match the generic connector's actual fallback, even when extra
		// dedicated-route targets were supplied. They can never replace it.
		store = strings.TrimSpace(strings.TrimPrefix(assetid.NativeCollectorAssetID(req.TargetID), "pbs-datastore-"))
		if store == "" {
			return nil, fmt.Errorf("PBS datastore is required")
		}
	}
	if store != "" {
		stores = append(stores, store)
	}
	targets := []string{parent}
	filters := []struct{ kind, store string }{{"storage-controller", ""}}
	seen := map[string]bool{}
	for _, store := range stores {
		if store == "" || strings.TrimSpace(store) != store {
			return nil, fmt.Errorf("invalid PBS datastore identity")
		}
		if !seen[store] {
			filters = append(filters, struct{ kind, store string }{"storage-pool", store})
			seen[store] = true
		}
	}
	for _, filter := range filters {
		rows, err := lookup.FindPBSActionAssets(collector, filter.kind, filter.store)
		if err != nil {
			return nil, err
		}
		if len(rows) > 2 {
			return nil, fmt.Errorf("PBS %s inventory is ambiguous", filter.kind)
		}
		for _, row := range rows {
			if err := e.verifyPBSOwner(row, collector); err != nil {
				return nil, err
			}
			targets = append(targets, row.ID)
		}
	}
	return targets, nil
}

func (e ConnectorEvaluator) verifyPBSOwner(asset assets.Asset, collector string) error {
	if asset.Source != "pbs" {
		return fmt.Errorf("PBS asset has a different source")
	}
	if _, scoped := assetid.CollectorScopeFromAssetID(asset.ID); scoped || strings.TrimSpace(asset.Metadata["collector_id"]) != "" {
		return verifyCollectorAsset(asset, collector)
	}
	// The dedicated runtime supports a unique legacy collector. Preserve that
	// case, but never assign an unowned legacy row across multiple appliances.
	selected, err := shared.ResolveEnabledHubCollector(e.Collectors, "pbs", "")
	if err != nil {
		return err
	}
	if selected.ID != collector {
		return fmt.Errorf("legacy PBS asset collector is ambiguous")
	}
	return nil
}

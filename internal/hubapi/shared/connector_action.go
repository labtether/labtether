package shared

import (
	"context"
	"fmt"
	"strings"

	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/persistence"
)

// Bind captures the provider and resource identity before a request is queued.
// Execute consumes that saved identity and rechecks current access/maintenance
// owners. Neither callback may replace the logical target used by the audit log.
type ConfiguredConnectorAction struct {
	Bind    func(context.Context, string, connectorsdk.ActionRequest) (connectorsdk.ActionRequest, error)
	Execute func(context.Context, string, connectorsdk.ActionRequest) (connectorsdk.ActionResult, error)
	// BindingParam names the saved runtime owner. Empty retains collector_id
	// for collector-backed providers; agent-backed Docker uses its actual agent.
	BindingParam string
	// UseCallerTimeout preserves a provider's existing execution bounds instead
	// of adding the worker's default one-minute timeout. Docker's coordinator
	// already bounds container and compose replies to 30 seconds and 5 minutes.
	UseCallerTimeout bool
}

// ResolveConnectorActionCollector reconciles a saved binding with inventory.
// Only a genuinely raw target can use an explicit provider without an asset
// owner; a stored unowned legacy row requires a unique enabled provider.
func ResolveConnectorActionCollector(assetStore persistence.AssetStore, collectorStore persistence.HubCollectorStore, kind string, req connectorsdk.ActionRequest) (*hubcollector.Collector, *assets.Asset, error) {
	if assetStore == nil {
		return nil, nil, fmt.Errorf("connector asset inventory unavailable")
	}
	kind = hubcollector.NormalizeCollectorType(kind)
	id := strings.TrimSpace(req.Params["collector_id"])
	target := strings.TrimSpace(req.TargetID)
	var stored *assets.Asset
	if target != "" {
		asset, found, err := assetStore.GetAsset(target)
		if err != nil {
			return nil, nil, err
		}
		if found {
			if !strings.EqualFold(strings.TrimSpace(asset.Source), kind) {
				return nil, nil, fmt.Errorf("action target belongs to a different provider")
			}
			owner := strings.TrimSpace(asset.Metadata["collector_id"])
			if id != "" && owner != "" && id != owner {
				return nil, nil, fmt.Errorf("action collector conflicts with its asset owner")
			}
			if id == "" {
				id = owner
			}
			stored = &asset
		}
	}
	selected, err := ResolveEnabledHubCollector(collectorStore, kind, id)
	if err != nil {
		return nil, nil, err
	}
	if scope, scoped := assetid.CollectorScopeFromAssetID(target); scoped {
		if scope != assetid.CollectorScope(selected.ID) {
			return nil, nil, fmt.Errorf("action asset namespace conflicts with its collector")
		}
	} else if stored != nil && strings.TrimSpace(stored.Metadata["collector_id"]) == "" && id != "" {
		unique, err := ResolveEnabledHubCollector(collectorStore, kind, "")
		if err != nil || unique.ID != selected.ID {
			return nil, nil, fmt.Errorf("legacy action asset ownership is ambiguous")
		}
	}
	return selected, stored, nil
}

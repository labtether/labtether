package homeassistantpkg

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/connectors/homeassistant"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/modelmap"
)

const (
	haBindingParam      = "_ha_binding"
	haEntityParam       = "_ha_entity_id"
	haTargetEntityParam = "_ha_target_entity"
	haTargetDomainParam = "_ha_target_domain"
)

var haEntityID = regexp.MustCompile(`^[a-z][a-z0-9_]*\.[a-z0-9_]+$`)

// BindAction preserves the audit target while saving an exact provider/entity
// identity. Empty service targets are deliberate, not a missing binding.
func (d *Deps) BindAction(ctx context.Context, actionID string, req connectorsdk.ActionRequest) (connectorsdk.ActionRequest, error) {
	params := make(map[string]string, len(req.Params)+4)
	for key, value := range req.Params {
		key = strings.TrimSpace(key)
		if strings.HasPrefix(key, "_ha_") {
			continue // A caller cannot supply a saved identity marker.
		}
		if _, duplicate := params[key]; key == "" || duplicate {
			return req, fmt.Errorf("invalid or duplicate Home Assistant parameter")
		}
		params[key] = strings.TrimSpace(value)
	}
	req.Params = params
	actionID, err := resolveHAAction(actionID, req)
	if err != nil {
		return req, err
	}
	collector, asset, entity, err := d.resolveHAActionOwner(ctx, req)
	if err != nil {
		return req, err
	}
	if actionID == "entity.toggle" && entity == "" {
		return req, fmt.Errorf("Home Assistant toggle requires an exact entity")
	}
	params["collector_id"], params[haBindingParam], params[haEntityParam] = collector.ID, "1", entity
	if asset != nil {
		params[haTargetEntityParam] = strings.TrimSpace(asset.Metadata["entity_id"])
		params[haTargetDomainParam] = strings.TrimSpace(asset.Metadata["domain"])
	}
	if err := d.checkHAActionMaintenance(ctx, actionID, req.TargetID, collector, entity); err != nil {
		return req, err
	}
	return req, ctx.Err()
}

func resolveHAAction(actionID string, req connectorsdk.ActionRequest) (string, error) {
	actionID = modelmap.ResolveActionID(actionID, req.TargetID, homeassistant.NewWithConfig(homeassistant.Config{}).Actions())
	switch actionID {
	case "entity.toggle":
		return actionID, nil
	case "service.call":
		if err := homeassistant.ValidateService(req.Params["service"]); err != nil {
			return "", err
		}
		return actionID, nil
	default:
		return "", fmt.Errorf("unsupported Home Assistant action")
	}
}

func (d *Deps) resolveHAActionOwner(ctx context.Context, req connectorsdk.ActionRequest) (*hubcollector.Collector, *assets.Asset, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, "", err
	}
	if d.AssetStore == nil {
		return nil, nil, "", fmt.Errorf("Home Assistant asset inventory unavailable")
	}
	target, owner := strings.TrimSpace(req.TargetID), strings.TrimSpace(req.Params["collector_id"])
	var stored *assets.Asset
	entity := target
	if target != "" {
		asset, found, err := d.AssetStore.GetAsset(target)
		if err != nil {
			return nil, nil, "", err
		}
		if found {
			if !isHomeAssistantEntity(asset) || asset.Type != "ha-entity" {
				return nil, nil, "", fmt.Errorf("action target is not a Home Assistant entity")
			}
			metadataOwner := strings.TrimSpace(asset.Metadata["collector_id"])
			if owner != "" && metadataOwner != "" && owner != metadataOwner {
				return nil, nil, "", fmt.Errorf("Home Assistant collector conflicts with target owner")
			}
			if owner == "" {
				owner = metadataOwner
			}
			stored, entity = &asset, strings.TrimSpace(asset.Metadata["entity_id"])
		} else if !haEntityID.MatchString(target) {
			return nil, nil, "", fmt.Errorf("Home Assistant asset is missing or raw entity ID is invalid")
		}
	}
	if entity != "" && !haEntityID.MatchString(entity) {
		return nil, nil, "", fmt.Errorf("Home Assistant raw entity identity is invalid")
	}
	collector, err := shared.ResolveEnabledHubCollector(d.HubCollectorStore, hubcollector.CollectorTypeHomeAssistant, owner)
	if err != nil {
		return nil, nil, "", err
	}
	if stored != nil {
		if err := d.validateHAEntityOwner(*stored, collector.ID); err != nil {
			return nil, nil, "", err
		}
	}
	return collector, stored, entity, ctx.Err()
}

func (d *Deps) validateHAEntityOwner(asset assets.Asset, collector string) error {
	if !isHomeAssistantEntity(asset) || asset.Type != "ha-entity" || !haEntityID.MatchString(strings.TrimSpace(asset.Metadata["entity_id"])) {
		return fmt.Errorf("Home Assistant entity identity is invalid")
	}
	owner := strings.TrimSpace(asset.Metadata["collector_id"])
	if owner != "" && owner != collector {
		return fmt.Errorf("Home Assistant entity belongs to another collector")
	}
	if scope, scoped := assetid.CollectorScopeFromAssetID(asset.ID); scoped {
		if !strings.EqualFold(scope, assetid.CollectorScope(collector)) {
			return fmt.Errorf("Home Assistant namespace conflicts with collector")
		}
	} else if owner == "" {
		unique, err := shared.ResolveEnabledHubCollector(d.HubCollectorStore, hubcollector.CollectorTypeHomeAssistant, "")
		if err != nil || unique.ID != collector {
			return fmt.Errorf("Home Assistant legacy entity ownership is ambiguous")
		}
	}
	return nil
}

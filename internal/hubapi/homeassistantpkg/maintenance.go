package homeassistantpkg

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/labtether/labtether/internal/assetid"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/hubapi/maintenanceguard"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/servicehttp"
)

func (d *Deps) enforceEntityMutation(ctx context.Context, w http.ResponseWriter, loaded assets.Asset, captured *runtime) bool {
	if ctx.Err() != nil {
		servicehttp.WriteError(w, http.StatusRequestTimeout, "Home Assistant action canceled before dispatch")
		return false
	}
	targets, err := d.entityMutationOwners(loaded, captured)
	if err != nil {
		servicehttp.WriteError(w, http.StatusConflict, "cannot verify Home Assistant action owners")
		return false
	}
	if ctx.Err() != nil {
		servicehttp.WriteError(w, http.StatusRequestTimeout, "Home Assistant action canceled before dispatch")
		return false
	}
	if err := maintenanceguard.CheckAssetActions(targets, d.EvaluateAssetGuardrails); err != nil {
		if !maintenanceguard.WriteBlockedAction(w, err) {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to evaluate maintenance windows")
		}
		return false
	}
	if ctx.Err() != nil {
		servicehttp.WriteError(w, http.StatusRequestTimeout, "Home Assistant action canceled before dispatch")
		return false
	}
	return true
}

// The entity and selected collector's parent are separate maintenance owners.
// Recheck the loaded entity before guarding so discovery cannot silently move
// this request to another entity or provider while its runtime is being loaded.
func (d *Deps) entityMutationOwners(loaded assets.Asset, captured *runtime) ([]string, error) {
	if d.AssetStore == nil || captured == nil || captured.CollectorID == "" {
		return nil, fmt.Errorf("Home Assistant action inventory unavailable")
	}
	current, found, err := d.AssetStore.GetAsset(loaded.ID)
	if err != nil || !found || !isHomeAssistantEntity(current) || current.Type != loaded.Type {
		return nil, fmt.Errorf("Home Assistant entity changed")
	}
	for _, key := range []string{"entity_id", "domain", "collector_id"} {
		if strings.TrimSpace(current.Metadata[key]) != strings.TrimSpace(loaded.Metadata[key]) {
			return nil, fmt.Errorf("Home Assistant entity identity changed")
		}
	}
	collector, err := shared.ResolveEnabledHubCollector(d.HubCollectorStore, hubcollector.CollectorTypeHomeAssistant, captured.CollectorID)
	if err != nil {
		return nil, err
	}
	owner := strings.TrimSpace(current.Metadata["collector_id"])
	if owner != "" && owner != collector.ID {
		return nil, fmt.Errorf("Home Assistant entity collector conflicts with runtime")
	}
	if scope, scoped := assetid.CollectorScopeFromAssetID(current.ID); scoped {
		if scope != assetid.CollectorScope(collector.ID) {
			return nil, fmt.Errorf("Home Assistant entity namespace conflicts with runtime")
		}
	} else if owner == "" {
		unique, err := shared.ResolveEnabledHubCollector(d.HubCollectorStore, hubcollector.CollectorTypeHomeAssistant, "")
		if err != nil || unique.ID != collector.ID {
			return nil, fmt.Errorf("Home Assistant legacy entity owner is ambiguous")
		}
	}
	return []string{current.ID, strings.TrimSpace(collector.AssetID)}, nil
}

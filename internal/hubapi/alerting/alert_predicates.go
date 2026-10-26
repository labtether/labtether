package alerting

import (
	"context"
	"fmt"
	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/persistence"
	"regexp"
	"strings"
)

type AlertPredicateContext struct {
	GroupIDs        map[string]struct{}
	ResourceKinds   map[string]struct{}
	ResourceClasses map[string]struct{}
	Capabilities    map[string]struct{}
}

var alertKindTokenSanitizePattern = regexp.MustCompile(`[^a-z0-9.\-]+`)

func NewAlertPredicateContext() AlertPredicateContext {
	return AlertPredicateContext{
		GroupIDs:        make(map[string]struct{}),
		ResourceKinds:   make(map[string]struct{}),
		ResourceClasses: make(map[string]struct{}),
		Capabilities:    make(map[string]struct{}),
	}
}

// resolveRuleTargetAssets expands an alert rule target set into concrete assets.
// It supports asset_id, group_id, selector, and optional global fallback for
// explicit global rules with an empty target set.
func (d *Deps) ResolveRuleTargetAssets(rule alerts.Rule, prefetchedAssets []assets.Asset, allowGlobalFallback bool) ([]assets.Asset, error) {
	return d.resolveRuleTargetAssetsWithCapabilities(rule, prefetchedAssets, nil, allowGlobalFallback)
}

func (d *Deps) resolveRuleTargetAssetsWithCapabilities(rule alerts.Rule, prefetchedAssets []assets.Asset, prefetchedCapabilities map[string][]string, allowGlobalFallback bool) ([]assets.Asset, error) {
	return d.resolveRuleTargetAssetsContext(context.Background(), rule, prefetchedAssets, prefetchedCapabilities, allowGlobalFallback)
}

func alertTargetsNeedInventory(rule alerts.Rule, allowGlobalFallback bool) bool {
	if len(rule.Targets) == 0 && allowGlobalFallback && alerts.NormalizeTargetScope(rule.TargetScope) == alerts.TargetScopeGlobal {
		return true
	}
	for _, target := range rule.Targets {
		if strings.TrimSpace(target.GroupID) != "" || len(target.Selector) > 0 {
			return true
		}
	}
	return false
}

func (d *Deps) resolveRuleTargetAssetsContext(
	ctx context.Context,
	rule alerts.Rule,
	prefetchedAssets []assets.Asset,
	prefetchedCapabilities map[string][]string,
	allowGlobalFallback bool,
) ([]assets.Asset, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if d.AssetStore == nil {
		return nil, nil
	}
	store, ok := d.AssetStore.(persistence.AlertAssetContextStore)
	if !ok {
		return nil, fmt.Errorf("context-aware alert asset lookup unavailable")
	}

	normalizedScope := alerts.NormalizeTargetScope(rule.TargetScope)
	targets := rule.Targets

	needsAllAssets := alertTargetsNeedInventory(rule, allowGlobalFallback)
	var explicitAssetIndexes map[string]int
	for _, target := range targets {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if id := strings.TrimSpace(target.AssetID); id != "" {
			if explicitAssetIndexes == nil {
				explicitAssetIndexes = make(map[string]int)
			}
			explicitAssetIndexes[id] = -1
		}
	}

	var allAssets []assets.Asset
	var allAssetsLoaded bool
	loadAllAssets := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if allAssetsLoaded {
			return nil
		}
		if prefetchedAssets != nil {
			// Target matching only reads this snapshot; the result is built separately.
			allAssets = prefetchedAssets
		} else {
			loaded, err := store.ListAssetsContext(ctx)
			if err != nil {
				return err
			}
			allAssets = loaded
		}
		if len(explicitAssetIndexes) > 0 {
			for index, entry := range allAssets {
				if err := ctx.Err(); err != nil {
					return err
				}
				id := strings.TrimSpace(entry.ID)
				if _, requested := explicitAssetIndexes[id]; requested {
					explicitAssetIndexes[id] = index // Preserve the last matching snapshot row.
				}
			}
		}
		allAssetsLoaded = true
		return nil
	}

	if needsAllAssets || prefetchedAssets != nil {
		if err := loadAllAssets(); err != nil {
			return nil, err
		}
	}

	outByID := make(map[string]assets.Asset, len(targets))
	addAsset := func(entry assets.Asset) {
		id := strings.TrimSpace(entry.ID)
		if id == "" {
			return
		}
		outByID[id] = entry
	}

	if len(targets) == 0 {
		if allowGlobalFallback && normalizedScope == alerts.TargetScopeGlobal {
			if err := loadAllAssets(); err != nil {
				return nil, err
			}
			for _, entry := range allAssets {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				addAsset(entry)
			}
		}
		return sortedAssetsByID(outByID), nil
	}

	for _, target := range targets {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if assetID := strings.TrimSpace(target.AssetID); assetID != "" {
			if index, found := explicitAssetIndexes[assetID]; found && index >= 0 {
				addAsset(allAssets[index])
				continue
			}
			entry, ok, err := store.GetAssetContext(ctx, assetID)
			if err != nil {
				return nil, err
			}
			if ok {
				addAsset(entry)
			}
			continue
		}

		if groupID := strings.TrimSpace(target.GroupID); groupID != "" {
			if err := loadAllAssets(); err != nil {
				return nil, err
			}
			for _, entry := range allAssets {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				if strings.EqualFold(strings.TrimSpace(entry.GroupID), groupID) {
					addAsset(entry)
				}
			}
			continue
		}

		if len(target.Selector) > 0 {
			if err := loadAllAssets(); err != nil {
				return nil, err
			}
			for start := 0; start < len(allAssets); start += persistence.AlertCapabilityBatchSize {
				end := min(start+persistence.AlertCapabilityBatchSize, len(allAssets))
				batch := allAssets[start:end]
				capabilities := prefetchedCapabilities
				if selectorNeedsAlertCapabilities(target.Selector) {
					var err error
					capabilities, err = d.alertCapabilityBatchContext(ctx, batch, prefetchedCapabilities)
					if err != nil {
						return nil, err
					}
				}
				for _, entry := range batch {
					if err := ctx.Err(); err != nil {
						return nil, err
					}
					if d.assetMatchesRuleSelectorWithCapabilities(entry, target.Selector, capabilities) {
						addAsset(entry)
					}
				}
			}
		}
	}

	return sortedAssetsByID(outByID), nil
}

func (d *Deps) BuildAlertPredicateContext(rule alerts.Rule, prefetchedAssets []assets.Asset) (AlertPredicateContext, error) {
	return d.buildAlertPredicateContextWithCapabilities(rule, prefetchedAssets, nil)
}

func (d *Deps) buildAlertPredicateContextWithCapabilities(
	rule alerts.Rule,
	prefetchedAssets []assets.Asset,
	prefetchedCapabilities map[string][]string,
) (AlertPredicateContext, error) {
	context := NewAlertPredicateContext()
	targetAssets, err := d.resolveRuleTargetAssetsWithCapabilities(rule, prefetchedAssets, prefetchedCapabilities, true)
	if err != nil {
		return context, err
	}

	for _, target := range rule.Targets {
		if groupID := strings.TrimSpace(target.GroupID); groupID != "" {
			context.GroupIDs[groupID] = struct{}{}
		}
	}

	for _, entry := range targetAssets {
		if groupID := strings.TrimSpace(entry.GroupID); groupID != "" {
			context.GroupIDs[groupID] = struct{}{}
		}

		for kind := range assetResourceKindCandidates(entry) {
			context.ResourceKinds[kind] = struct{}{}
		}
		for class := range assetResourceClassCandidates(entry) {
			context.ResourceClasses[class] = struct{}{}
		}
		for capability := range d.assetCapabilitySetWithPrefetch(entry, prefetchedCapabilities) {
			context.Capabilities[capability] = struct{}{}
		}
	}

	return context, nil
}

func (d *Deps) assetMatchesRuleSelectorWithCapabilities(
	entry assets.Asset,
	selector map[string]any,
	prefetchedCapabilities map[string][]string,
) bool {
	if len(selector) == 0 {
		return false
	}

	kindCandidates := assetResourceKindCandidates(entry)
	classCandidates := assetResourceClassCandidates(entry)
	var capabilities map[string]struct{}

	for rawKey, rawValue := range selector {
		key := strings.ToLower(strings.TrimSpace(rawKey))
		if key == "" {
			continue
		}
		if _, deprecated := DeprecatedCanonicalPredicateKeys[key]; deprecated {
			return false
		}
		expected := selectorStringValues(rawValue)

		switch key {
		case "resource_kind":
			if !setContainsAny(kindCandidates, normalizeSelectorValues(expected, normalizeKindToken)) {
				return false
			}
		case "resource_class":
			if !setContainsAny(classCandidates, normalizeSelectorValues(expected, normalizeSelectorToken)) {
				return false
			}
		case "capability":
			if capabilities == nil {
				capabilities = d.assetCapabilitySetWithPrefetch(entry, prefetchedCapabilities)
			}
			if !setContainsAny(capabilities, normalizeSelectorValues(expected, normalizeSelectorToken)) {
				return false
			}
		case "capabilities_any":
			if capabilities == nil {
				capabilities = d.assetCapabilitySetWithPrefetch(entry, prefetchedCapabilities)
			}
			if !setContainsAny(capabilities, normalizeSelectorValues(expected, normalizeSelectorToken)) {
				return false
			}
		case "capabilities_all":
			if capabilities == nil {
				capabilities = d.assetCapabilitySetWithPrefetch(entry, prefetchedCapabilities)
			}
			if !setContainsAll(capabilities, normalizeSelectorValues(expected, normalizeSelectorToken)) {
				return false
			}
		case "id", "asset_id":
			if !valueMatchesExpected(entry.ID, expected) {
				return false
			}
		case "name":
			if !valueMatchesExpected(entry.Name, expected) {
				return false
			}
		case "group_id":
			if !valueMatchesExpected(entry.GroupID, expected) {
				return false
			}
		case "source":
			if !valueMatchesExpected(entry.Source, expected) {
				return false
			}
		case "status":
			if !valueMatchesExpected(entry.Status, expected) {
				return false
			}
		case "platform":
			if !valueMatchesExpected(entry.Platform, expected) {
				return false
			}
		default:
			if strings.HasPrefix(key, "metadata.") {
				metadataKey := strings.TrimSpace(strings.TrimPrefix(key, "metadata."))
				if metadataKey == "" || !valueMatchesExpected(entry.Metadata[metadataKey], expected) {
					return false
				}
				continue
			}
			if strings.HasPrefix(key, "attribute.") || strings.HasPrefix(key, "attributes.") {
				attributePath := key
				if strings.HasPrefix(attributePath, "attribute.") {
					attributePath = strings.TrimPrefix(attributePath, "attribute.")
				} else {
					attributePath = strings.TrimPrefix(attributePath, "attributes.")
				}
				if !attributePathMatchesExpected(entry.Attributes, attributePath, expected) {
					return false
				}
				continue
			}
			if metadataValue, ok := entry.Metadata[key]; ok {
				if !valueMatchesExpected(metadataValue, expected) {
					return false
				}
				continue
			}
			if !attributePathMatchesExpected(entry.Attributes, key, expected) {
				return false
			}
		}
	}

	return true
}

func (d *Deps) assetCapabilitySetWithPrefetch(
	entry assets.Asset,
	prefetchedCapabilities map[string][]string,
) map[string]struct{} {
	capabilityIDs := d.inferCapabilityIDsFromAssetMetadata(entry)
	prefetchedAssetID := strings.TrimSpace(entry.ID)
	if prefetchedCapabilities != nil {
		if prefetched, ok := prefetchedCapabilities[prefetchedAssetID]; ok {
			capabilityIDs = d.mergeCapabilityIDs(capabilityIDs, prefetched)
		}
	} else if d.CanonicalStore != nil {
		if capabilitySet, ok, err := d.CanonicalStore.GetCapabilitySet("resource", strings.TrimSpace(entry.ID)); err == nil && ok {
			capabilityIDs = d.mergeCapabilityIDs(capabilityIDs, d.capabilityIDsFromSet(capabilitySet))
		}
	}

	out := make(map[string]struct{}, len(capabilityIDs))
	for _, capabilityID := range capabilityIDs {
		normalized := normalizeSelectorToken(capabilityID)
		if normalized == "" {
			continue
		}
		out[normalized] = struct{}{}
	}
	return out
}

func selectorNeedsAlertCapabilities(selector map[string]any) bool {
	for key := range selector {
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "capability", "capabilities_any", "capabilities_all":
			return true
		}
	}
	return false
}

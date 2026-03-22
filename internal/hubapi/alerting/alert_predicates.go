package alerting

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/model"
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

func (d *Deps) resolveRuleTargetAssetsWithCapabilities(
	rule alerts.Rule,
	prefetchedAssets []assets.Asset,
	prefetchedCapabilities map[string][]string,
	allowGlobalFallback bool,
) ([]assets.Asset, error) {
	if d.AssetStore == nil {
		return nil, nil
	}

	normalizedScope := alerts.NormalizeTargetScope(rule.TargetScope)
	targets := rule.Targets

	needsAllAssets := len(targets) == 0 && allowGlobalFallback && normalizedScope == alerts.TargetScopeGlobal
	for _, target := range targets {
		if strings.TrimSpace(target.GroupID) != "" || len(target.Selector) > 0 {
			needsAllAssets = true
			break
		}
	}

	var allAssets []assets.Asset
	var allAssetsByID map[string]assets.Asset
	var allAssetsLoaded bool
	loadAllAssets := func() error {
		if allAssetsLoaded {
			return nil
		}
		if prefetchedAssets != nil {
			allAssets = append([]assets.Asset(nil), prefetchedAssets...)
		} else {
			loaded, err := d.AssetStore.ListAssets()
			if err != nil {
				return err
			}
			allAssets = loaded
		}
		allAssetsByID = make(map[string]assets.Asset, len(allAssets))
		for _, entry := range allAssets {
			id := strings.TrimSpace(entry.ID)
			if id == "" {
				continue
			}
			allAssetsByID[id] = entry
		}
		allAssetsLoaded = true
		return nil
	}

	if needsAllAssets {
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
				addAsset(entry)
			}
		}
		return sortedAssetsByID(outByID), nil
	}

	for _, target := range targets {
		if assetID := strings.TrimSpace(target.AssetID); assetID != "" {
			if allAssetsByID != nil {
				if entry, ok := allAssetsByID[assetID]; ok {
					addAsset(entry)
					continue
				}
			}
			entry, ok, err := d.AssetStore.GetAsset(assetID)
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
			for _, entry := range allAssets {
				if d.assetMatchesRuleSelectorWithCapabilities(entry, target.Selector, prefetchedCapabilities) {
					addAsset(entry)
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

func assetResourceKindCandidates(entry assets.Asset) map[string]struct{} {
	out := make(map[string]struct{}, 3)
	addKindCandidate := func(value string) {
		normalized := normalizeKindToken(value)
		if normalized == "" {
			return
		}
		out[normalized] = struct{}{}
	}

	addKindCandidate(entry.ResourceKind)
	addKindCandidate(entry.Metadata["resource_kind"])
	addKindCandidate(entry.Type)

	return out
}

func assetResourceClassCandidates(entry assets.Asset) map[string]struct{} {
	out := make(map[string]struct{}, 3)
	addClass := func(value string) {
		normalized := normalizeSelectorToken(value)
		if normalized == "" {
			return
		}
		out[normalized] = struct{}{}
	}

	addClass(entry.ResourceClass)
	addClass(entry.Metadata["resource_class"])

	derivedKind := normalizeKindToken(firstNonEmptyString(entry.ResourceKind, entry.Metadata["resource_kind"], entry.Type))
	if derivedKind != "" {
		derivedClass := string(model.ResourceClassForKind(derivedKind))
		addClass(derivedClass)
	}

	return out
}

func selectorStringValues(value any) []string {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return nil
		}
		if strings.Contains(trimmed, ",") {
			parts := strings.Split(trimmed, ",")
			out := make([]string, 0, len(parts))
			for _, part := range parts {
				if normalized := strings.TrimSpace(part); normalized != "" {
					out = append(out, normalized)
				}
			}
			return out
		}
		return []string{trimmed}
	case []string:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if normalized := strings.TrimSpace(item); normalized != "" {
				out = append(out, normalized)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			out = append(out, selectorStringValues(item)...)
		}
		return out
	case bool:
		if typed {
			return []string{"true"}
		}
		return []string{"false"}
	default:
		return []string{strings.TrimSpace(fmt.Sprint(typed))}
	}
}

func normalizeSelectorValues(values []string, normalizer func(string) string) []string {
	if len(values) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		normalized := normalizer(value)
		if normalized == "" {
			continue
		}
		if _, exists := set[normalized]; exists {
			continue
		}
		set[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

func setContainsAny(set map[string]struct{}, expected []string) bool {
	if len(expected) == 0 {
		return false
	}
	for _, value := range expected {
		if _, ok := set[value]; ok {
			return true
		}
	}
	return false
}

func setContainsAll(set map[string]struct{}, expected []string) bool {
	if len(expected) == 0 {
		return false
	}
	for _, value := range expected {
		if _, ok := set[value]; !ok {
			return false
		}
	}
	return true
}

func valueMatchesExpected(candidate string, expected []string) bool {
	normalizedCandidate := normalizeSelectorToken(candidate)
	if normalizedCandidate == "" {
		return false
	}
	for _, value := range normalizeSelectorValues(expected, normalizeSelectorToken) {
		if normalizedCandidate == value {
			return true
		}
	}
	return false
}

func attributePathMatchesExpected(attributes map[string]any, path string, expected []string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	rawValue, ok := readAttributePath(attributes, path)
	if !ok {
		return false
	}
	candidate := normalizeSelectorToken(fmt.Sprint(rawValue))
	if candidate == "" {
		return false
	}
	for _, value := range normalizeSelectorValues(expected, normalizeSelectorToken) {
		if value == candidate {
			return true
		}
	}
	return false
}

func readAttributePath(attributes map[string]any, path string) (any, bool) {
	if len(attributes) == 0 {
		return nil, false
	}
	segments := strings.Split(path, ".")
	current := any(attributes)
	for _, segment := range segments {
		key := strings.TrimSpace(segment)
		if key == "" {
			return nil, false
		}
		switch typed := current.(type) {
		case map[string]any:
			next, ok := typed[key]
			if !ok {
				return nil, false
			}
			current = next
		case map[string]string:
			next, ok := typed[key]
			if !ok {
				return nil, false
			}
			current = next
		case []any:
			index, err := strconv.Atoi(key)
			if err != nil || index < 0 || index >= len(typed) {
				return nil, false
			}
			current = typed[index]
		default:
			return nil, false
		}
	}
	return current, true
}

func normalizeSelectorToken(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeKindToken(value string) string {
	normalized := normalizeSelectorToken(value)
	if normalized == "" {
		return ""
	}
	normalized = strings.ReplaceAll(normalized, "_", "-")
	normalized = strings.ReplaceAll(normalized, " ", "-")
	normalized = alertKindTokenSanitizePattern.ReplaceAllString(normalized, "-")
	normalized = strings.Trim(normalized, "-")
	return normalized
}

func sortedAssetsByID(values map[string]assets.Asset) []assets.Asset {
	if len(values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	out := make([]assets.Asset, 0, len(keys))
	for _, key := range keys {
		out = append(out, values[key])
	}
	return out
}

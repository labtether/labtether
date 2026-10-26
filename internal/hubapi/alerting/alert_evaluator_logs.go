package alerting

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/persistence"
)

var (
	alertRegexMu    sync.RWMutex
	alertRegexCache = make(map[string]*regexp.Regexp, 128)
)

func cachedAlertRegexp(pattern string) (*regexp.Regexp, error) {
	alertRegexMu.RLock()
	if re, ok := alertRegexCache[pattern]; ok {
		alertRegexMu.RUnlock()
		return re, nil
	}
	alertRegexMu.RUnlock()

	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}

	alertRegexMu.Lock()
	if len(alertRegexCache) >= 256 {
		// Evict all — simple reset; regex compile is cheap enough
		alertRegexCache = make(map[string]*regexp.Regexp, 128)
	}
	alertRegexCache[pattern] = re
	alertRegexMu.Unlock()
	return re, nil
}

func (d *Deps) evaluateLogPatternOutcome(
	ctx context.Context,
	rule alerts.Rule,
	prefetchedAssets []assets.Asset,
	prefetchedCapabilities map[string][]string,
) (alertConditionOutcome, error) {
	outcome := newAlertConditionOutcome()
	if d.LogStore == nil {
		if err := ctx.Err(); err != nil {
			return alertConditionOutcome{}, err
		}
		return outcome.result()
	}

	store, ok := d.LogStore.(persistence.AlertLogContextStore)
	if !ok {
		return outcome, fmt.Errorf("context-aware alert log query unavailable")
	}
	windowSec := rule.WindowSeconds
	if windowSec <= 0 {
		windowSec = alerts.DefaultWindowSeconds
	}
	now := time.Now().UTC()
	windowStart := now.Add(-alerts.DurationFromSeconds(windowSec, time.Duration(alerts.DefaultWindowSeconds)*time.Second))

	pattern, _ := rule.Condition["pattern"].(string)
	if pattern == "" {
		if err := ctx.Err(); err != nil {
			return alertConditionOutcome{}, err
		}
		return outcome.result()
	}

	re, err := cachedAlertRegexp(pattern)
	if err != nil {
		return alertConditionOutcome{}, fmt.Errorf("invalid log pattern: %w", err)
	}

	minOccurrences := 1
	if mo, ok := rule.Condition["min_occurrences"]; ok {
		if v, ok := toFloat64(mo); ok {
			minOccurrences = int(v)
			if minOccurrences < 1 {
				minOccurrences = 1
			}
		}
	}

	sourceFilter := strings.ToLower(strings.TrimSpace(conditionString(rule.Condition, "source")))
	levelFilter := strings.ToLower(strings.TrimSpace(conditionString(rule.Condition, "level")))
	fieldEquals := conditionStringMap(rule.Condition["field_equals"])
	requiresFields := len(fieldEquals) > 0

	matchesEvent := func(event logs.Event) bool {
		if !re.MatchString(event.Message) {
			return false
		}
		for key, expectedValue := range fieldEquals {
			if !strings.EqualFold(strings.TrimSpace(event.Fields[key]), expectedValue) {
				return false
			}
		}
		return true
	}

	normalizedScope := alerts.NormalizeTargetScope(rule.TargetScope)
	if normalizedScope == alerts.TargetScopeGlobal && len(rule.Targets) == 0 {
		events, err := store.QueryEventsContext(ctx, logs.QueryRequest{
			From:          windowStart,
			To:            now,
			Source:        sourceFilter,
			Level:         levelFilter,
			Limit:         500,
			ExcludeFields: !requiresFields,
		})
		if err != nil {
			return alertConditionOutcome{}, fmt.Errorf("global log query: %w", err)
		}
		matchCount := 0
		matchedAssets := map[string]bool{}
		for _, event := range events {
			if err := ctx.Err(); err != nil {
				return alertConditionOutcome{}, err
			}
			if matchesEvent(event) {
				matchCount++
				if event.AssetID != "" {
					matchedAssets[event.AssetID] = true
				}
			}
		}
		if matchCount >= minOccurrences {
			outcome.Triggered = true
			// One shared budget covers all optional group reads, not one budget
			// per matching row. Missing groups never erase exact log asset IDs.
			groupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			byID := make(map[string]assets.Asset, len(prefetchedAssets))
			for _, asset := range prefetchedAssets {
				byID[asset.ID] = asset
			}
			lookup, canRead := d.AssetStore.(persistence.AssetContextStore)
			for assetID := range matchedAssets {
				asset := assets.Asset{ID: assetID}
				if prefetchedAssets != nil {
					if known, ok := byID[assetID]; ok {
						asset = known
					}
				} else if canRead && groupCtx.Err() == nil {
					loaded, found, loadErr := lookup.GetAssetContext(groupCtx, assetID)
					if loadErr == nil && found && loaded.ID == assetID && groupCtx.Err() == nil {
						asset = loaded
					}
				}
				outcome.witnessAsset(asset)
			}
		}
		if err := ctx.Err(); err != nil {
			return alertConditionOutcome{}, err
		}
		return outcome.result()
	}

	targetAssets, err := d.resolveRuleTargetAssetsContext(ctx, rule, prefetchedAssets, prefetchedCapabilities, true)
	if err != nil {
		return alertConditionOutcome{}, err
	}
	assetIDs := uniqueAlertAssetIDs(targetAssets)
	limit := 500 * len(assetIDs)
	if limit < 500 {
		limit = 500
	}
	if limit > 5000 {
		limit = 5000
	}
	events, err := store.QueryEventsContext(ctx, logs.QueryRequest{
		GroupAssetIDs: assetIDs,
		From:          windowStart,
		To:            now,
		Source:        sourceFilter,
		Level:         levelFilter,
		Limit:         limit,
		ExcludeFields: !requiresFields,
	})
	if err != nil {
		return alertConditionOutcome{}, fmt.Errorf("log query for targeted assets: %w", err)
	}
	matchCounts := make(map[string]int, len(assetIDs))
	for _, event := range events {
		if err := ctx.Err(); err != nil {
			return alertConditionOutcome{}, err
		}
		assetID := strings.TrimSpace(event.AssetID)
		if assetID == "" {
			continue
		}
		if !matchesEvent(event) {
			continue
		}
		matchCounts[assetID]++

	}
	for _, asset := range targetAssets {
		if matchCounts[asset.ID] >= minOccurrences {
			outcome.witnessAsset(asset)
		}
	}
	if err := ctx.Err(); err != nil {
		return alertConditionOutcome{}, err
	}
	return outcome.result()
}

func (d *Deps) EvaluateLogPatternWithPrefetch(rule alerts.Rule, prefetchedAssets []assets.Asset, prefetchedCapabilities map[string][]string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	outcome, err := d.evaluateLogPatternOutcome(ctx, rule, prefetchedAssets, prefetchedCapabilities)
	return outcome.Triggered, err
}

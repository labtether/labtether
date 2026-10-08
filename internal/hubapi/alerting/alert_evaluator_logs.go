package alerting

import (
	"fmt"
	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/logs"
	"regexp"
	"strings"
	"sync"
	"time"
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

func (d *Deps) EvaluateLogPatternWithPrefetch(
	rule alerts.Rule,
	prefetchedAssets []assets.Asset,
	prefetchedCapabilities map[string][]string,
) (bool, error) {
	if d.LogStore == nil {
		return false, nil
	}

	windowSec := rule.WindowSeconds
	if windowSec <= 0 {
		windowSec = alerts.DefaultWindowSeconds
	}
	now := time.Now().UTC()
	windowStart := now.Add(-alerts.DurationFromSeconds(windowSec, time.Duration(alerts.DefaultWindowSeconds)*time.Second))

	pattern, _ := rule.Condition["pattern"].(string)
	if pattern == "" {
		return false, nil
	}

	re, err := cachedAlertRegexp(pattern)
	if err != nil {
		return false, fmt.Errorf("invalid log pattern: %w", err)
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

	matchesWindowEvents := func(events []logs.Event) bool {
		matchCount := 0
		for _, event := range events {
			if matchesEvent(event) {
				matchCount++
				if matchCount >= minOccurrences {
					return true
				}
			}
		}
		return false
	}

	normalizedScope := alerts.NormalizeTargetScope(rule.TargetScope)
	if normalizedScope == alerts.TargetScopeGlobal && len(rule.Targets) == 0 {
		events, err := d.LogStore.QueryEvents(logs.QueryRequest{
			From:          windowStart,
			To:            now,
			Source:        sourceFilter,
			Level:         levelFilter,
			Limit:         500,
			ExcludeFields: !requiresFields,
		})
		if err != nil {
			return false, fmt.Errorf("global log query: %w", err)
		}
		return matchesWindowEvents(events), nil
	}

	targetAssets, err := d.resolveRuleTargetAssetsWithCapabilities(rule, prefetchedAssets, prefetchedCapabilities, true)
	if err != nil {
		return false, err
	}
	assetIDs := uniqueAlertAssetIDs(targetAssets)
	limit := 500 * len(assetIDs)
	if limit < 500 {
		limit = 500
	}
	if limit > 5000 {
		limit = 5000
	}
	events, err := d.LogStore.QueryEvents(logs.QueryRequest{
		GroupAssetIDs: assetIDs,
		From:          windowStart,
		To:            now,
		Source:        sourceFilter,
		Level:         levelFilter,
		Limit:         limit,
		ExcludeFields: !requiresFields,
	})
	if err != nil {
		return false, fmt.Errorf("log query for targeted assets: %w", err)
	}
	matchCounts := make(map[string]int, len(assetIDs))
	for _, event := range events {
		assetID := strings.TrimSpace(event.AssetID)
		if assetID == "" {
			continue
		}
		if !matchesEvent(event) {
			continue
		}
		matchCounts[assetID]++
		if matchCounts[assetID] >= minOccurrences {
			return true, nil
		}
	}
	return false, nil
}

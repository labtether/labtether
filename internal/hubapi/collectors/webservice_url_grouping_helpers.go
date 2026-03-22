package collectors

import (
	"fmt"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
)

type WebServiceURLGroupingMode string

const (
	WebServiceURLGroupingModeOff          WebServiceURLGroupingMode = "off"
	WebServiceURLGroupingModeConservative WebServiceURLGroupingMode = "conservative"
	WebServiceURLGroupingModeBalanced     WebServiceURLGroupingMode = "balanced"
	WebServiceURLGroupingModeAggressive   WebServiceURLGroupingMode = "aggressive"
	defaultWebServiceURLGroupingCacheTTL                            = 5 * time.Second
)

type WebServiceURLGroupingConfig struct {
	Mode                WebServiceURLGroupingMode
	DryRun              bool
	ConfidenceThreshold int
	AliasRules          []WebServiceAliasRule
	ForceGroupRules     map[string]struct{}
	NeverGroupRules     map[string]struct{}
}

type WebServiceAliasRule struct {
	fromLabels []string
	toLabels   []string
}

func applyWebServiceURLGrouping(in []agentmgr.DiscoveredWebService, cfg WebServiceURLGroupingConfig) ([]agentmgr.DiscoveredWebService, []WebServiceGroupingSuggestion) {
	if len(in) <= 1 {
		return in, nil
	}
	if cfg.Mode == WebServiceURLGroupingModeOff || cfg.DryRun {
		return in, nil
	}

	var suggestions []WebServiceGroupingSuggestion
	out := make([]agentmgr.DiscoveredWebService, 0, len(in))
	groupingBuckets := make(map[string][]int, len(in))
	for i, svc := range in {
		cloned := cloneWebServiceForGrouping(svc)
		key := webServiceGroupingKey(cloned, cfg)
		grouped := false
		if len(cfg.ForceGroupRules) > 0 {
			for idx := range out {
				if !shouldForceGroupDiscoveredWebServices(out[idx], cloned, cfg) {
					continue
				}
				out[idx] = groupDiscoveredWebServiceURLs(out[idx], cloned)
				appendGroupingBucketIndex(groupingBuckets, key, idx)
				grouped = true
				break
			}
		}
		if grouped {
			continue
		}
		if key == "" {
			out = append(out, cloned)
			continue
		}
		candidates := groupingBuckets[key]
		for _, idx := range candidates {
			if canGroupDiscoveredWebServices(out[idx], cloned, cfg) {
				out[idx] = groupDiscoveredWebServiceURLs(out[idx], cloned)
				grouped = true
				break
			}
			// Check for below-threshold-but-plausible match (suggestion).
			if canSuggestGrouping(out[idx], cloned, cfg) {
				suggestions = append(suggestions, WebServiceGroupingSuggestion{
					ID:              fmt.Sprintf("suggest-%d-%d", idx, i),
					BaseServiceURL:  out[idx].URL,
					BaseServiceName: out[idx].Name,
					BaseIconKey:     out[idx].IconKey,
					SuggestedURL:    cloned.URL,
					Confidence:      webServiceGroupingConfidence(out[idx], cloned, cfg),
				})
			}
		}
		if grouped {
			continue
		}
		appendGroupingBucketIndex(groupingBuckets, key, len(out))
		out = append(out, cloned)
	}
	return out, suggestions
}

func appendGroupingBucketIndex(groupingBuckets map[string][]int, key string, idx int) {
	if strings.TrimSpace(key) == "" {
		return
	}
	candidates := groupingBuckets[key]
	for _, existing := range candidates {
		if existing == idx {
			return
		}
	}
	groupingBuckets[key] = append(candidates, idx)
}

func shouldForceGroupDiscoveredWebServices(base, incoming agentmgr.DiscoveredWebService, cfg WebServiceURLGroupingConfig) bool {
	if len(cfg.ForceGroupRules) == 0 {
		return false
	}
	if !sameWebServiceGroupingHost(base, incoming) {
		return false
	}
	if webServiceNeverGroupRuleApplies(base, incoming, cfg.NeverGroupRules) {
		return false
	}
	return webServiceNeverGroupRuleApplies(base, incoming, cfg.ForceGroupRules)
}

func canGroupDiscoveredWebServices(base, incoming agentmgr.DiscoveredWebService, cfg WebServiceURLGroupingConfig) bool {
	if !sameWebServiceGroupingHost(base, incoming) {
		return false
	}
	if webServiceNeverGroupRuleApplies(base, incoming, cfg.NeverGroupRules) {
		return false
	}
	if webServiceNeverGroupRuleApplies(base, incoming, cfg.ForceGroupRules) {
		return true
	}
	confidence := webServiceGroupingConfidence(base, incoming, cfg)
	return confidence >= cfg.ConfidenceThreshold
}

// canSuggestGrouping returns true when a pair is below the grouping confidence
// threshold but still plausible enough (>= suggestionMinConfidence) to offer
// as a suggestion to the operator.
func canSuggestGrouping(base, incoming agentmgr.DiscoveredWebService, cfg WebServiceURLGroupingConfig) bool {
	if !sameWebServiceGroupingHost(base, incoming) {
		return false
	}
	if webServiceNeverGroupRuleApplies(base, incoming, cfg.NeverGroupRules) {
		return false
	}
	confidence := webServiceGroupingConfidence(base, incoming, cfg)
	return confidence >= suggestionMinConfidence && confidence < cfg.ConfidenceThreshold
}

func sameWebServiceGroupingHost(base, incoming agentmgr.DiscoveredWebService) bool {
	return strings.EqualFold(
		strings.TrimSpace(base.HostAssetID),
		strings.TrimSpace(incoming.HostAssetID),
	)
}

func webServiceNeverGroupRuleApplies(base, incoming agentmgr.DiscoveredWebService, rules map[string]struct{}) bool {
	if len(rules) == 0 {
		return false
	}

	baseIDs := webServiceURLIdentities(base)
	incomingIDs := webServiceURLIdentities(incoming)
	if len(baseIDs) == 0 || len(incomingIDs) == 0 {
		return false
	}
	for _, left := range baseIDs {
		for _, right := range incomingIDs {
			key := webServiceGroupingPairKey(left, right)
			if key == "" {
				continue
			}
			if _, matched := rules[key]; matched {
				return true
			}
		}
	}
	return false
}

func webServiceGroupingConfidence(base, incoming agentmgr.DiscoveredWebService, cfg WebServiceURLGroupingConfig) int {
	if sharedContainerIdentity(base, incoming) {
		return 100
	}
	if sharedMetadataURLIdentity(base, incoming, "backend_url") {
		return 100
	}
	if sharedMetadataURLIdentity(base, incoming, "raw_url") {
		return 98
	}

	baseIDs := webServiceURLIdentitiesWithAliasRules(base, cfg.AliasRules)
	incomingIDs := webServiceURLIdentitiesWithAliasRules(incoming, cfg.AliasRules)
	if !sharesAnyURLIdentity(baseIDs, incomingIDs) {
		return 0
	}

	confidence := 90
	switch cfg.Mode {
	case WebServiceURLGroupingModeConservative:
		confidence = 95
	case WebServiceURLGroupingModeBalanced:
		confidence = 90
	case WebServiceURLGroupingModeAggressive:
		confidence = 85
	}

	baseHint := normalizedWebServiceGroupingHint(base)
	incomingHint := normalizedWebServiceGroupingHint(incoming)
	if baseHint != "" && incomingHint != "" && baseHint == incomingHint {
		confidence += 5
	}
	if confidence > 100 {
		confidence = 100
	}
	return confidence
}

func sharedContainerIdentity(base, incoming agentmgr.DiscoveredWebService) bool {
	baseID := strings.ToLower(strings.TrimSpace(base.ContainerID))
	incomingID := strings.ToLower(strings.TrimSpace(incoming.ContainerID))
	return baseID != "" && baseID == incomingID
}

func sharedMetadataURLIdentity(base, incoming agentmgr.DiscoveredWebService, key string) bool {
	if base.Metadata == nil || incoming.Metadata == nil {
		return false
	}
	left := canonicalWebServiceURLIdentity(base.Metadata[key], nil)
	right := canonicalWebServiceURLIdentity(incoming.Metadata[key], nil)
	return left != "" && left == right
}

func normalizedWebServiceGroupingHint(svc agentmgr.DiscoveredWebService) string {
	serviceHint := strings.ToLower(strings.TrimSpace(svc.ServiceKey))
	if serviceHint != "" {
		return serviceHint
	}
	return strings.ToLower(strings.TrimSpace(svc.Name))
}

func webServiceGroupingKey(svc agentmgr.DiscoveredWebService, cfg WebServiceURLGroupingConfig) string {
	hostAssetID := strings.ToLower(strings.TrimSpace(svc.HostAssetID))
	if hostAssetID == "" {
		return ""
	}

	if svc.Metadata != nil {
		if backendKey := canonicalWebServiceURLIdentity(svc.Metadata["backend_url"], nil); backendKey != "" {
			return "backend|" + hostAssetID + "|" + backendKey
		}
		if rawKey := canonicalWebServiceURLIdentity(svc.Metadata["raw_url"], nil); rawKey != "" {
			return "raw|" + hostAssetID + "|" + rawKey
		}
	}

	if containerID := strings.ToLower(strings.TrimSpace(svc.ContainerID)); containerID != "" {
		return "container|" + hostAssetID + "|" + containerID
	}

	if len(cfg.AliasRules) == 0 {
		return ""
	}

	aliasURLKey := canonicalWebServiceURLIdentity(svc.URL, cfg.AliasRules)
	if aliasURLKey == "" {
		return ""
	}

	serviceHint := normalizedWebServiceGroupingHint(svc)
	switch cfg.Mode {
	case WebServiceURLGroupingModeConservative:
		if serviceHint == "" {
			return ""
		}
		return "alias|" + hostAssetID + "|" + aliasURLKey + "|" + serviceHint
	case WebServiceURLGroupingModeBalanced:
		return "alias|" + hostAssetID + "|" + aliasURLKey + "|" + serviceHint
	case WebServiceURLGroupingModeAggressive:
		return "alias|" + hostAssetID + "|" + aliasURLKey
	default:
		return ""
	}
}

package discovery

import (
	"net"
	"strings"
)

// MatchCandidate represents a proposed edge or composite from a matcher.
type MatchCandidate struct {
	SourceAssetID string
	TargetAssetID string
	Type          string // "edge" or "composite"
	EdgeType      string // relationship_type for edges (e.g., "contains")
	Confidence    float64
	Signal        string // description of what matched
}

// Matcher produces match candidates from a set of asset signals.
type Matcher interface {
	Match(signals []AssetSignals) []MatchCandidate
}

// ParentHintMatcher resolves parent hints (node, agent_id, endpoint_id,
// collector_id) by finding an asset whose name or ID matches the hint value.
// Produces "edge" candidates with EdgeType "contains" or "runs_on".
type ParentHintMatcher struct {
	// AllAssets provides name/ID lookup across the full asset set.
	// Keys are lowercased asset names and IDs mapping to asset IDs.
	AllAssets []AssetData
}

func (m *ParentHintMatcher) Match(signals []AssetSignals) []MatchCandidate {
	// Build a lookup from lowercased name/ID to asset ID.
	nameToID := make(map[string]string, len(m.AllAssets)*2)
	for _, a := range m.AllAssets {
		nameToID[strings.ToLower(a.ID)] = a.ID
		nameToID[strings.ToLower(a.Name)] = a.ID
	}

	var candidates []MatchCandidate
	for _, sig := range signals {
		for hintKey, hintVal := range sig.ParentHints {
			lower := strings.ToLower(hintVal)
			parentID, ok := nameToID[lower]
			if !ok {
				continue
			}
			// Avoid self-loops.
			if parentID == sig.AssetID {
				continue
			}
			edgeType := "contains"
			if hintKey == "agent_id" || hintKey == "endpoint_id" {
				edgeType = "runs_on"
			}
			candidates = append(candidates, MatchCandidate{
				SourceAssetID: parentID,
				TargetAssetID: sig.AssetID,
				Type:          "edge",
				EdgeType:      edgeType,
				Confidence:    0.95,
				Signal:        "parent_hint:" + hintKey + "=" + hintVal,
			})
		}
	}
	return candidates
}

// IPMatcher groups signals by IP address. When two signals from different
// sources share a non-trivial IP, it proposes a composite candidate.
// Confidence: 0.95 for specific private IPs, 0.85 for more common ranges.
type IPMatcher struct{}

func (m *IPMatcher) Match(signals []AssetSignals) []MatchCandidate {
	// ip → list of signals that carry it
	type entry struct {
		assetID string
		source  string
	}
	ipIndex := make(map[string][]entry)
	for _, sig := range signals {
		for _, ip := range sig.IPs {
			ipIndex[ip] = append(ipIndex[ip], entry{sig.AssetID, sig.Source})
		}
	}

	seen := make(map[[2]string]bool)
	var candidates []MatchCandidate
	for ip, entries := range ipIndex {
		for i := 0; i < len(entries); i++ {
			for j := i + 1; j < len(entries); j++ {
				a, b := entries[i], entries[j]
				// Skip pairs from the same source.
				if a.source == b.source {
					continue
				}
				// Canonical pair key (sorted) to avoid duplicates.
				key := pairKey(a.assetID, b.assetID)
				if seen[key] {
					continue
				}
				seen[key] = true

				conf := ipConfidence(ip)
				candidates = append(candidates, MatchCandidate{
					SourceAssetID: a.assetID,
					TargetAssetID: b.assetID,
					Type:          "composite",
					Confidence:    conf,
					Signal:        "shared_ip:" + ip,
				})
			}
		}
	}
	return candidates
}

// ipConfidence returns 0.95 for specific private IPs (not in common class C
// subnets like 192.168.1.x or 10.0.0.x) and 0.85 for more common ranges.
func ipConfidence(ipStr string) float64 {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return 0.85
	}
	// Common default router/host subnets get lower confidence because many
	// devices share these addresses across isolated networks.
	commonPrefixes := []string{
		"192.168.1.", "192.168.0.",
		"10.0.0.", "10.0.1.",
		"172.16.0.",
	}
	for _, prefix := range commonPrefixes {
		if strings.HasPrefix(ipStr, prefix) {
			return 0.85
		}
	}
	return 0.95
}

// HostnameMatcher groups signals by hostname. Exact hostname matches across
// different sources produce composite candidates with confidence 0.90.
type HostnameMatcher struct{}

func (m *HostnameMatcher) Match(signals []AssetSignals) []MatchCandidate {
	type entry struct {
		assetID string
		source  string
	}
	hostnameIndex := make(map[string][]entry)
	for _, sig := range signals {
		for _, h := range sig.Hostnames {
			lower := strings.ToLower(h)
			hostnameIndex[lower] = append(hostnameIndex[lower], entry{sig.AssetID, sig.Source})
		}
	}

	seen := make(map[[2]string]bool)
	var candidates []MatchCandidate
	for hostname, entries := range hostnameIndex {
		for i := 0; i < len(entries); i++ {
			for j := i + 1; j < len(entries); j++ {
				a, b := entries[i], entries[j]
				if a.source == b.source {
					continue
				}
				key := pairKey(a.assetID, b.assetID)
				if seen[key] {
					continue
				}
				seen[key] = true
				candidates = append(candidates, MatchCandidate{
					SourceAssetID: a.assetID,
					TargetAssetID: b.assetID,
					Type:          "composite",
					Confidence:    0.90,
					Signal:        "shared_hostname:" + hostname,
				})
			}
		}
	}
	return candidates
}

// NameTokenMatcher groups signals by shared non-generic name tokens across
// different sources. Confidence scales from 0.60 to 0.80 based on the number
// of shared tokens.
type NameTokenMatcher struct{}

func (m *NameTokenMatcher) Match(signals []AssetSignals) []MatchCandidate {
	type entry struct {
		assetID string
		source  string
		tokens  map[string]bool
	}
	// Build token → entries index.
	type assetEntry struct {
		assetID string
		source  string
		tokens  map[string]bool
	}
	tokenIndex := make(map[string][]int) // token → slice indices into entries
	entries := make([]assetEntry, 0, len(signals))
	for _, sig := range signals {
		if len(sig.NameTokens) == 0 {
			continue
		}
		idx := len(entries)
		tokSet := make(map[string]bool, len(sig.NameTokens))
		for _, t := range sig.NameTokens {
			tokSet[t] = true
			tokenIndex[t] = append(tokenIndex[t], idx)
		}
		entries = append(entries, assetEntry{sig.AssetID, sig.Source, tokSet})
	}

	// For each pair of assets from different sources, count shared tokens.
	sharedCount := make(map[[2]string]int)
	pairSources := make(map[[2]string][2]string) // track sources for dedup
	for _, idxList := range tokenIndex {
		for i := 0; i < len(idxList); i++ {
			for j := i + 1; j < len(idxList); j++ {
				a, b := entries[idxList[i]], entries[idxList[j]]
				if a.source == b.source {
					continue
				}
				key := pairKey(a.assetID, b.assetID)
				sharedCount[key]++
				pairSources[key] = [2]string{a.assetID, b.assetID}
			}
		}
	}

	var candidates []MatchCandidate
	for key, count := range sharedCount {
		if count == 0 {
			continue
		}
		ids := pairSources[key]
		conf := tokenConfidence(count)
		candidates = append(candidates, MatchCandidate{
			SourceAssetID: ids[0],
			TargetAssetID: ids[1],
			Type:          "composite",
			Confidence:    conf,
			Signal:        "shared_name_tokens",
		})
	}
	return candidates
}

// tokenConfidence maps shared token count to confidence in [0.60, 0.80].
func tokenConfidence(sharedCount int) float64 {
	switch {
	case sharedCount >= 3:
		return 0.80
	case sharedCount == 2:
		return 0.70
	default:
		return 0.60
	}
}

// StructuralMatcher uses type-based inference to suggest containment edges.
// When a service-type asset (TrueNAS, PBS, HA) and a host-type asset share the
// same source scope but have no existing link, it proposes a containment edge.
// Confidence: 0.50–0.65 depending on specificity.
type StructuralMatcher struct{}

// serviceTypes is the set of asset types treated as "hosted services" that
// imply a parent host.
var serviceTypes = map[string]bool{
	"truenas":        true,
	"proxmox_backup": true,
	"pbs":            true,
	"homeassistant":  true,
	"home_assistant": true,
	"ha":             true,
	"portainer":      true,
	"grafana":        true,
	"nextcloud":      true,
}

// hostTypes is the set of asset types treated as potential parent hosts.
var hostTypes = map[string]bool{
	"host":   true,
	"node":   true,
	"server": true,
	"linux":  true,
	"vm":     true,
	"lxc":    true,
}

func (m *StructuralMatcher) Match(signals []AssetSignals) []MatchCandidate {
	// Bucket signals by source scope.
	type entry struct {
		assetID string
		typ     string
	}
	bySource := make(map[string][]entry)
	for _, sig := range signals {
		bySource[sig.Source] = append(bySource[sig.Source], entry{sig.AssetID, strings.ToLower(sig.Type)})
	}

	var candidates []MatchCandidate
	for _, entries := range bySource {
		var services, hosts []entry
		for _, e := range entries {
			if serviceTypes[e.typ] {
				services = append(services, e)
			} else if hostTypes[e.typ] {
				hosts = append(hosts, e)
			}
		}
		// Suggest each service is contained by each host in the same source scope.
		for _, svc := range services {
			for _, h := range hosts {
				if svc.assetID == h.assetID {
					continue
				}
				conf := 0.55
				if len(hosts) == 1 {
					// Only one candidate host — more confident.
					conf = 0.65
				}
				candidates = append(candidates, MatchCandidate{
					SourceAssetID: h.assetID,
					TargetAssetID: svc.assetID,
					Type:          "edge",
					EdgeType:      "contains",
					Confidence:    conf,
					Signal:        "structural:" + h.typ + "_hosts_" + svc.typ,
				})
			}
		}
	}
	return candidates
}

// pairKey returns a canonical, order-independent key for two asset IDs.
func pairKey(a, b string) [2]string {
	if a < b {
		return [2]string{a, b}
	}
	return [2]string{b, a}
}

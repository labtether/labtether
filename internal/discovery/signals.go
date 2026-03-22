package discovery

import (
	"net"
	"strings"
)

// AssetData is a normalized view of a discovered infrastructure object,
// populated by source-specific connectors before signal extraction.
type AssetData struct {
	ID       string
	Name     string
	Source   string
	Type     string
	Host     string
	Metadata map[string]string
}

// AssetSignals holds the extracted, normalized signals for a single asset.
// These signals are consumed by the grouping engine to correlate assets
// across sources.
type AssetSignals struct {
	AssetID     string
	IPs         []string
	Hostnames   []string
	MACs        []string
	NameTokens  []string
	Source      string
	Type        string
	ParentHints map[string]string
}

// genericTokens lists words that appear in many asset names but carry no
// useful identity signal for correlation.
var genericTokens = map[string]bool{
	"vm": true, "host": true, "server": true, "node": true,
	"container": true, "docker": true, "portainer": true,
	"proxmox": true, "agent": true, "service": true,
	"device": true, "machine": true, "instance": true,
}

// excludedIPs lists addresses that are never meaningful correlation signals.
var excludedIPs = map[string]bool{
	"127.0.0.1": true, "::1": true, "0.0.0.0": true,
}

// ExtractSignals derives IPs, hostnames, name tokens, and parent hints from
// an AssetData record. The returned AssetSignals are ready for use by the
// grouping engine's correlation logic.
func ExtractSignals(a AssetData) AssetSignals {
	s := AssetSignals{
		AssetID:     a.ID,
		Source:      a.Source,
		Type:        a.Type,
		ParentHints: make(map[string]string),
	}

	// Extract IPs/hostnames from the Host field.
	if a.Host != "" {
		ip := net.ParseIP(a.Host)
		if ip != nil && !excludedIPs[a.Host] && !isDockerBridge(a.Host) {
			s.IPs = append(s.IPs, a.Host)
		} else if ip == nil {
			s.Hostnames = append(s.Hostnames, strings.ToLower(a.Host))
		}
	}

	// Extract additional IPs from well-known metadata keys.
	for _, key := range []string{"ip", "address", "host_ip", "management_ip"} {
		if v, ok := a.Metadata[key]; ok && v != "" {
			ip := net.ParseIP(v)
			if ip != nil && !excludedIPs[v] && !isDockerBridge(v) {
				s.IPs = append(s.IPs, v)
			}
		}
	}

	// Extract parent/collector hints for hierarchy inference.
	for _, key := range []string{"node", "agent_id", "endpoint_id", "collector_id", "host_id"} {
		if v, ok := a.Metadata[key]; ok && v != "" {
			s.ParentHints[key] = v
		}
	}

	s.NameTokens = tokenizeName(a.Name)
	return s
}

// tokenizeName lowercases a name and splits it on common delimiters, then
// removes tokens that are too short or listed in genericTokens.
func tokenizeName(name string) []string {
	parts := strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
		return r == ' ' || r == '-' || r == '_' || r == '.' || r == '(' || r == ')'
	})
	return filterGenericTokens(parts)
}

// filterGenericTokens removes empty, single-character, and generic tokens
// from a token list, returning only tokens with identity value.
func filterGenericTokens(tokens []string) []string {
	var result []string
	for _, t := range tokens {
		if t == "" || len(t) < 2 {
			continue
		}
		if !genericTokens[t] {
			result = append(result, t)
		}
	}
	return result
}

// isDockerBridge reports whether an IP address belongs to a Docker default
// bridge network range (172.17.x.x or 172.18.x.x), which are container-local
// and not useful for cross-source correlation.
func isDockerBridge(ip string) bool {
	return strings.HasPrefix(ip, "172.17.") || strings.HasPrefix(ip, "172.18.")
}

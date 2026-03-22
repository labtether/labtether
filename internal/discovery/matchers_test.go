package discovery

import (
	"testing"
)

// --- ParentHintMatcher ---

func TestParentHintMatcher(t *testing.T) {
	allAssets := []AssetData{
		{ID: "asset-pve-1", Name: "proxmox1", Source: "proxmox", Type: "node"},
		{ID: "asset-vm-1", Name: "OmegaNAS VM", Source: "proxmox", Type: "vm"},
	}
	signals := []AssetSignals{
		{
			AssetID:     "asset-vm-1",
			Source:      "proxmox",
			Type:        "vm",
			ParentHints: map[string]string{"node": "proxmox1"},
		},
	}

	m := &ParentHintMatcher{AllAssets: allAssets}
	candidates := m.Match(signals)

	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d: %v", len(candidates), candidates)
	}
	c := candidates[0]
	if c.SourceAssetID != "asset-pve-1" {
		t.Errorf("expected parent asset-pve-1, got %s", c.SourceAssetID)
	}
	if c.TargetAssetID != "asset-vm-1" {
		t.Errorf("expected child asset-vm-1, got %s", c.TargetAssetID)
	}
	if c.EdgeType != "contains" {
		t.Errorf("expected EdgeType contains, got %s", c.EdgeType)
	}
	if c.Type != "edge" {
		t.Errorf("expected Type edge, got %s", c.Type)
	}
	if c.Confidence != 0.95 {
		t.Errorf("expected confidence 0.95, got %f", c.Confidence)
	}
}

func TestParentHintMatcher_AgentEdgeType(t *testing.T) {
	allAssets := []AssetData{
		{ID: "agent-host-1", Name: "myhost", Source: "agent", Type: "host"},
	}
	signals := []AssetSignals{
		{
			AssetID:     "svc-1",
			Source:      "agent",
			Type:        "service",
			ParentHints: map[string]string{"agent_id": "myhost"},
		},
	}
	m := &ParentHintMatcher{AllAssets: allAssets}
	candidates := m.Match(signals)

	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
	if candidates[0].EdgeType != "runs_on" {
		t.Errorf("expected runs_on for agent_id hint, got %s", candidates[0].EdgeType)
	}
}

func TestParentHintMatcher_NoMatch(t *testing.T) {
	allAssets := []AssetData{
		{ID: "asset-1", Name: "somehost", Source: "proxmox", Type: "node"},
	}
	signals := []AssetSignals{
		{
			AssetID:     "asset-2",
			Source:      "proxmox",
			Type:        "vm",
			ParentHints: map[string]string{"node": "unknownhost"},
		},
	}
	m := &ParentHintMatcher{AllAssets: allAssets}
	candidates := m.Match(signals)
	if len(candidates) != 0 {
		t.Errorf("expected no candidates for unresolvable hint, got %d", len(candidates))
	}
}

// --- IPMatcher ---

func TestIPMatcher_DifferentSources(t *testing.T) {
	signals := []AssetSignals{
		{AssetID: "a1", Source: "proxmox", IPs: []string{"10.0.5.50"}},
		{AssetID: "a2", Source: "truenas", IPs: []string{"10.0.5.50"}},
	}
	m := &IPMatcher{}
	candidates := m.Match(signals)

	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d: %v", len(candidates), candidates)
	}
	c := candidates[0]
	if c.Type != "composite" {
		t.Errorf("expected composite, got %s", c.Type)
	}
	if c.Confidence != 0.95 {
		t.Errorf("expected confidence 0.95 for specific IP, got %f", c.Confidence)
	}
}

func TestIPMatcher_SameSource_NoMatch(t *testing.T) {
	signals := []AssetSignals{
		{AssetID: "a1", Source: "proxmox", IPs: []string{"10.0.5.50"}},
		{AssetID: "a2", Source: "proxmox", IPs: []string{"10.0.5.50"}},
	}
	m := &IPMatcher{}
	candidates := m.Match(signals)
	if len(candidates) != 0 {
		t.Errorf("expected no candidates for same-source pair, got %d", len(candidates))
	}
}

func TestIPMatcher_CommonRangeLowerConfidence(t *testing.T) {
	signals := []AssetSignals{
		{AssetID: "a1", Source: "proxmox", IPs: []string{"192.168.1.100"}},
		{AssetID: "a2", Source: "truenas", IPs: []string{"192.168.1.100"}},
	}
	m := &IPMatcher{}
	candidates := m.Match(signals)
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
	if candidates[0].Confidence != 0.85 {
		t.Errorf("expected confidence 0.85 for common IP range, got %f", candidates[0].Confidence)
	}
}

func TestIPMatcher_NoDuplicateCandidates(t *testing.T) {
	// Same pair, two shared IPs — should still be one candidate per unique pair
	// (different IPs produce separate candidates since they are separate signals).
	signals := []AssetSignals{
		{AssetID: "a1", Source: "proxmox", IPs: []string{"10.0.5.50", "10.0.5.51"}},
		{AssetID: "a2", Source: "truenas", IPs: []string{"10.0.5.50", "10.0.5.51"}},
	}
	m := &IPMatcher{}
	candidates := m.Match(signals)
	// Each IP produces one candidate, but pairKey deduplication should leave 1.
	if len(candidates) != 1 {
		t.Errorf("expected 1 deduplicated candidate for same asset pair, got %d", len(candidates))
	}
}

// --- HostnameMatcher ---

func TestHostnameMatcher_ExactMatch(t *testing.T) {
	signals := []AssetSignals{
		{AssetID: "a1", Source: "proxmox", Hostnames: []string{"omeganas"}},
		{AssetID: "a2", Source: "truenas", Hostnames: []string{"omeganas"}},
	}
	m := &HostnameMatcher{}
	candidates := m.Match(signals)

	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d: %v", len(candidates), candidates)
	}
	c := candidates[0]
	if c.Type != "composite" {
		t.Errorf("expected composite, got %s", c.Type)
	}
	if c.Confidence != 0.90 {
		t.Errorf("expected confidence 0.90, got %f", c.Confidence)
	}
}

func TestHostnameMatcher_SameSource_NoMatch(t *testing.T) {
	signals := []AssetSignals{
		{AssetID: "a1", Source: "proxmox", Hostnames: []string{"omeganas"}},
		{AssetID: "a2", Source: "proxmox", Hostnames: []string{"omeganas"}},
	}
	m := &HostnameMatcher{}
	candidates := m.Match(signals)
	if len(candidates) != 0 {
		t.Errorf("expected no candidates for same-source pair, got %d", len(candidates))
	}
}

func TestHostnameMatcher_CaseInsensitive(t *testing.T) {
	signals := []AssetSignals{
		{AssetID: "a1", Source: "proxmox", Hostnames: []string{"OmegaNAS"}},
		{AssetID: "a2", Source: "truenas", Hostnames: []string{"omeganas"}},
	}
	m := &HostnameMatcher{}
	candidates := m.Match(signals)
	if len(candidates) != 1 {
		t.Errorf("expected 1 candidate for case-insensitive match, got %d", len(candidates))
	}
}

// --- NameTokenMatcher ---

func TestNameTokenMatcher_SharedToken(t *testing.T) {
	signals := []AssetSignals{
		{
			AssetID:    "a1",
			Source:     "proxmox",
			NameTokens: []string{"omeganas"},
		},
		{
			AssetID:    "a2",
			Source:     "truenas",
			NameTokens: []string{"omeganas"},
		},
	}
	m := &NameTokenMatcher{}
	candidates := m.Match(signals)

	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d: %v", len(candidates), candidates)
	}
	c := candidates[0]
	if c.Type != "composite" {
		t.Errorf("expected composite, got %s", c.Type)
	}
	if c.Confidence != 0.60 {
		t.Errorf("expected confidence 0.60 for 1 shared token, got %f", c.Confidence)
	}
}

func TestNameTokenMatcher_MultipleSharedTokens(t *testing.T) {
	signals := []AssetSignals{
		{
			AssetID:    "a1",
			Source:     "proxmox",
			NameTokens: []string{"omeganas", "backup", "storage"},
		},
		{
			AssetID:    "a2",
			Source:     "truenas",
			NameTokens: []string{"omeganas", "backup", "storage"},
		},
	}
	m := &NameTokenMatcher{}
	candidates := m.Match(signals)

	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
	if candidates[0].Confidence != 0.80 {
		t.Errorf("expected confidence 0.80 for 3 shared tokens, got %f", candidates[0].Confidence)
	}
}

func TestNameTokenMatcher_SameSource_NoMatch(t *testing.T) {
	signals := []AssetSignals{
		{AssetID: "a1", Source: "proxmox", NameTokens: []string{"omeganas"}},
		{AssetID: "a2", Source: "proxmox", NameTokens: []string{"omeganas"}},
	}
	m := &NameTokenMatcher{}
	candidates := m.Match(signals)
	if len(candidates) != 0 {
		t.Errorf("expected no candidates for same-source pair, got %d", len(candidates))
	}
}

func TestNameTokenMatcher_NoSharedTokens(t *testing.T) {
	signals := []AssetSignals{
		{AssetID: "a1", Source: "proxmox", NameTokens: []string{"alphanas"}},
		{AssetID: "a2", Source: "truenas", NameTokens: []string{"betaserver"}},
	}
	m := &NameTokenMatcher{}
	candidates := m.Match(signals)
	if len(candidates) != 0 {
		t.Errorf("expected no candidates for non-overlapping tokens, got %d", len(candidates))
	}
}

// --- StructuralMatcher ---

func TestStructuralMatcher_TrueNASNearHost(t *testing.T) {
	signals := []AssetSignals{
		{AssetID: "host-1", Source: "agent", Type: "host"},
		{AssetID: "nas-1", Source: "agent", Type: "truenas"},
	}
	m := &StructuralMatcher{}
	candidates := m.Match(signals)

	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d: %v", len(candidates), candidates)
	}
	c := candidates[0]
	if c.SourceAssetID != "host-1" {
		t.Errorf("expected parent host-1, got %s", c.SourceAssetID)
	}
	if c.TargetAssetID != "nas-1" {
		t.Errorf("expected child nas-1, got %s", c.TargetAssetID)
	}
	if c.EdgeType != "contains" {
		t.Errorf("expected EdgeType contains, got %s", c.EdgeType)
	}
	if c.Type != "edge" {
		t.Errorf("expected Type edge, got %s", c.Type)
	}
	// Single host → confidence 0.65.
	if c.Confidence != 0.65 {
		t.Errorf("expected confidence 0.65 for single host, got %f", c.Confidence)
	}
}

func TestStructuralMatcher_MultipleHosts_LowerConfidence(t *testing.T) {
	signals := []AssetSignals{
		{AssetID: "host-1", Source: "agent", Type: "host"},
		{AssetID: "host-2", Source: "agent", Type: "host"},
		{AssetID: "nas-1", Source: "agent", Type: "truenas"},
	}
	m := &StructuralMatcher{}
	candidates := m.Match(signals)

	// Two hosts → two candidates, both with confidence 0.55.
	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(candidates))
	}
	for _, c := range candidates {
		if c.Confidence != 0.55 {
			t.Errorf("expected confidence 0.55 for multiple hosts, got %f", c.Confidence)
		}
	}
}

func TestStructuralMatcher_DifferentSources_NoMatch(t *testing.T) {
	// Service and host in different source scopes → no structural match.
	signals := []AssetSignals{
		{AssetID: "host-1", Source: "agent_a", Type: "host"},
		{AssetID: "nas-1", Source: "agent_b", Type: "truenas"},
	}
	m := &StructuralMatcher{}
	candidates := m.Match(signals)
	if len(candidates) != 0 {
		t.Errorf("expected no candidates across different source scopes, got %d", len(candidates))
	}
}

func TestStructuralMatcher_HomeAssistant(t *testing.T) {
	signals := []AssetSignals{
		{AssetID: "linux-1", Source: "agent", Type: "linux"},
		{AssetID: "ha-1", Source: "agent", Type: "homeassistant"},
	}
	m := &StructuralMatcher{}
	candidates := m.Match(signals)
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate for HA near host, got %d", len(candidates))
	}
	if candidates[0].SourceAssetID != "linux-1" || candidates[0].TargetAssetID != "ha-1" {
		t.Errorf("unexpected candidate: %+v", candidates[0])
	}
}

// --- tokenConfidence ---

func TestTokenConfidence(t *testing.T) {
	cases := []struct {
		count    int
		expected float64
	}{
		{1, 0.60},
		{2, 0.70},
		{3, 0.80},
		{10, 0.80},
	}
	for _, tc := range cases {
		got := tokenConfidence(tc.count)
		if got != tc.expected {
			t.Errorf("tokenConfidence(%d) = %f, want %f", tc.count, got, tc.expected)
		}
	}
}

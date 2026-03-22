package discovery

import "testing"

func TestExtractSignals(t *testing.T) {
	asset := AssetData{
		ID:     "vm-1",
		Name:   "OmegaNAS VM",
		Source: "proxmox",
		Type:   "vm",
		Host:   "10.0.1.50",
		Metadata: map[string]string{
			"node": "proxmox1",
		},
	}
	signals := ExtractSignals(asset)

	if len(signals.IPs) == 0 || signals.IPs[0] != "10.0.1.50" {
		t.Errorf("expected IP 10.0.1.50, got %v", signals.IPs)
	}
	if len(signals.NameTokens) == 0 {
		t.Error("expected name tokens")
	}
	found := false
	for _, tok := range signals.NameTokens {
		if tok == "omeganas" {
			found = true
		}
		if tok == "vm" {
			t.Error("generic token 'vm' should be filtered")
		}
	}
	if !found {
		t.Error("expected token 'omeganas'")
	}
	if signals.ParentHints["node"] != "proxmox1" {
		t.Errorf("expected parent hint node=proxmox1, got %v", signals.ParentHints)
	}
}

func TestFilterGenericTokens(t *testing.T) {
	tokens := filterGenericTokens([]string{"omeganas", "vm", "host", "server", "nas01"})
	if len(tokens) != 2 {
		t.Errorf("expected 2 tokens, got %d: %v", len(tokens), tokens)
	}
}

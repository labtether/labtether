package resources

import "testing"

func TestProviderScopeForGroupUsesSiteScope(t *testing.T) {
	if got := ProviderScopeForGroup("group-123"); string(got) != "site" {
		t.Fatalf("expected grouped provider scope to persist as site, got %q", got)
	}
	if got := ProviderScopeForGroup(""); string(got) != "global" {
		t.Fatalf("expected empty group scope to remain global, got %q", got)
	}
}

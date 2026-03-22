package apikeys

import "testing"

func TestScopeMatches(t *testing.T) {
	tests := []struct {
		name     string
		granted  []string
		required string
		want     bool
	}{
		{"exact match", []string{"assets:read"}, "assets:read", true},
		{"no match", []string{"assets:read"}, "assets:write", false},
		{"wildcard scope", []string{"assets:*"}, "assets:read", true},
		{"wildcard scope write", []string{"assets:*"}, "assets:write", true},
		{"global wildcard", []string{"*"}, "docker:write", true},
		{"multiple scopes", []string{"assets:read", "files:write"}, "files:write", true},
		{"empty granted", []string{}, "assets:read", false},
		{"empty required", []string{"assets:read"}, "", false},
		{"category wildcard no match", []string{"assets:*"}, "files:read", false},
		{"bare category", []string{"assets"}, "assets:read", true},
		{"bare category no match", []string{"assets"}, "files:read", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ScopeAllows(tt.granted, tt.required); got != tt.want {
				t.Errorf("ScopeAllows(%v, %q) = %v, want %v", tt.granted, tt.required, got, tt.want)
			}
		})
	}
}

func TestAssetAllowed(t *testing.T) {
	tests := []struct {
		name          string
		allowedAssets []string
		assetID       string
		want          bool
	}{
		{"empty list allows all", []string{}, "server1", true},
		{"nil list allows all", nil, "server1", true},
		{"exact match", []string{"server1", "server2"}, "server1", true},
		{"not in list", []string{"server1", "server2"}, "server3", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AssetAllowed(tt.allowedAssets, tt.assetID); got != tt.want {
				t.Errorf("AssetAllowed(%v, %q) = %v, want %v", tt.allowedAssets, tt.assetID, got, tt.want)
			}
		})
	}
}

func TestValidateScopes(t *testing.T) {
	if err := ValidateScopes([]string{"assets:read", "files:*", "*"}); err != nil {
		t.Errorf("valid scopes should not error: %v", err)
	}
	if err := ValidateScopes([]string{"bogus"}); err == nil {
		t.Error("invalid scope should error")
	}
	if err := ValidateScopes([]string{}); err == nil {
		t.Error("empty scopes should error")
	}
}

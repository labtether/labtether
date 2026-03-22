package remoteaccess

import (
	"encoding/base64"
	"testing"
)

func TestTokenAllowsAnyCapability(t *testing.T) {
	token := "header." + base64.RawURLEncoding.EncodeToString([]byte(`{"capabilities":["agent.command.execute","agent.update.apply"]}`)) + ".sig"

	checked, allowed := TokenAllowsAnyCapability(token, "agent.command.execute")
	if !checked {
		t.Fatalf("expected capabilities to be parsed from token")
	}
	if !allowed {
		t.Fatalf("expected token capability check to allow command execution")
	}
	checked, allowed = TokenAllowsAnyCapability(token, "missing.capability")
	if !checked {
		t.Fatalf("expected capabilities to be parsed from token")
	}
	if allowed {
		t.Fatalf("expected missing capability to be denied")
	}
}

func TestTokenAllowsAnyCapabilityUnknownTokenFormat(t *testing.T) {
	checked, allowed := TokenAllowsAnyCapability("opaque-token", "agent.command.execute")
	if checked {
		t.Fatalf("expected opaque token to skip capability claim enforcement")
	}
	if !allowed {
		t.Fatalf("expected opaque token to be treated as legacy-allowed")
	}
}

func TestValidateUpdatePackages(t *testing.T) {
	if err := ValidateUpdatePackages([]string{"curl", "openssl-dev"}); err != nil {
		t.Fatalf("expected valid package names, got %v", err)
	}
	if err := ValidateUpdatePackages([]string{"bad;rm -rf /"}); err == nil {
		t.Fatalf("expected invalid package name to be rejected")
	}
}

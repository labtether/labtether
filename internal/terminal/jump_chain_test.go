package terminal

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestDecodeJumpChainNormalizesAndBounds(t *testing.T) {
	chain, err := DecodeJumpChain(json.RawMessage(`{"hops":[{"host":" 2001:db8::1 ","port":0,"username":" root ","credential_profile_id":" profile-1 "}]}`))
	if err != nil {
		t.Fatalf("DecodeJumpChain: %v", err)
	}
	if got := chain.Hops[0]; got.Host != "2001:db8::1" || got.Port != 22 || got.Username != "root" || got.CredentialProfileID != "profile-1" {
		t.Fatalf("unexpected normalized hop: %#v", got)
	}

	hops := make([]string, MaxJumpChainHops+1)
	for i := range hops {
		hops[i] = fmt.Sprintf(`{"host":"host-%d","username":"root","credential_profile_id":"cred-%d"}`, i, i)
	}
	if _, err := DecodeJumpChain(json.RawMessage(`{"hops":[` + strings.Join(hops, ",") + `]}`)); err == nil || !strings.Contains(err.Error(), "maximum") {
		t.Fatalf("expected hop-limit error, got %v", err)
	}
}

func TestDecodeJumpChainRejectsMalformedSecurityFields(t *testing.T) {
	tests := []string{
		`{"hops":[{"host":"host/path","username":"root","credential_profile_id":"cred"}]}`,
		`{"hops":[{"host":"host","port":65536,"username":"root","credential_profile_id":"cred"}]}`,
		`{"hops":[{"host":"host","username":"root\n","credential_profile_id":"cred"}]}`,
		`{"hops":[{"host":"host","username":"root","credential_profile_id":""}]}`,
		`{"hops":[{"host":"host","username":"root","credential_profile_id":"cred","unknown":true}]}`,
		`{"hops":[{"host":"host","username":"root","credential_profile_id":"cred"},{"host":"host","port":22,"username":"root","credential_profile_id":"cred"}]}`,
	}
	for _, raw := range tests {
		if _, err := DecodeJumpChain(json.RawMessage(raw)); err == nil {
			t.Fatalf("expected validation error for %s", raw)
		}
	}
}

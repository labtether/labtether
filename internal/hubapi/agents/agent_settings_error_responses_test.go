package agents

import "testing"

func TestSafeAgentSettingsApplyError(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "remote overrides", raw: "remote overrides are disabled on this agent", want: "remote overrides are disabled on this agent"},
		{name: "identity", raw: "fingerprint mismatch for settings apply", want: "agent identity changed; reconnect the agent before applying settings"},
		{name: "known local setting", raw: "setting tls_ca_file is local-only", want: "setting tls_ca_file is local-only"},
		{name: "arbitrary detail", raw: "write /etc/labtether/config: token=secret-value", want: "agent failed to apply settings"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := safeAgentSettingsApplyError(tc.raw); got != tc.want {
				t.Fatalf("safeAgentSettingsApplyError(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestLatestAgentVersionForPlatformHandlesMissingCache(t *testing.T) {
	d := &Deps{}
	if _, _, err := d.LatestAgentVersionForPlatform("linux", "amd64"); err == nil {
		t.Fatal("expected missing agent cache error")
	}
}

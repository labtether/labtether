package collectors

import "testing"

// Portainer collectors carry an admin API key or password; they must verify the
// server certificate unless the operator explicitly opted out.
func TestPortainerCollectorVerifiesTLSByDefault(t *testing.T) {
	cases := []struct {
		name   string
		config map[string]any
		want   bool
	}{
		{name: "unset", config: map[string]any{"base_url": "https://portainer.lan:9443"}, want: false},
		{name: "nil config", config: nil, want: false},
		{name: "unparseable", config: map[string]any{"skip_verify": "maybe"}, want: false},
		{name: "explicit false", config: map[string]any{"skip_verify": false}, want: false},
		{name: "explicit true bool", config: map[string]any{"skip_verify": true}, want: true},
		{name: "explicit true string", config: map[string]any{"skip_verify": "true"}, want: true},
	}
	for _, tc := range cases {
		if got := portainerCollectorSkipVerify(tc.config); got != tc.want {
			t.Errorf("%s: skip verify = %t, want %t", tc.name, got, tc.want)
		}
	}
}

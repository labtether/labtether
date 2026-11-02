package agents

import "testing"

// The Hub pushes a self-update to every agent marked "update_available" and
// shows the console's update notice. Text comparison marked a newer agent as
// needing the Hub's older release, and v1.10.0 sorts below v1.9.0 as text.
func TestDetermineAgentVersionStatusComparesReleasesByNumber(t *testing.T) {
	cases := []struct {
		current, latest, want string
	}{
		{"v1.7.1", "v2.0.0", "update_available"},
		{"v1.9.0", "v1.10.0", "update_available"},
		{"v2.0.0", "v2.0.0", "up_to_date"},
		{"2.0.0", "v2.0.0", "up_to_date"},
		{"v2.0.1", "v2.0.0", "up_to_date"},
		{"v1.10.0", "v1.9.0", "up_to_date"},
		{"v2.0.0-rc1", "v2.0.0", "update_available"},
		{"git:0123456789ab", "v2.0.0", "update_available"},
		{"", "v2.0.0", "unknown"},
	}
	for _, tc := range cases {
		if got := DetermineAgentVersionStatus(tc.current, tc.latest); got != tc.want {
			t.Errorf("DetermineAgentVersionStatus(%q, %q) = %q, want %q", tc.current, tc.latest, got, tc.want)
		}
	}
}

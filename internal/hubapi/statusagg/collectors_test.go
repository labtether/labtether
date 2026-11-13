package statusagg

import (
	"testing"
	"time"

	"github.com/labtether/labtether/internal/assets"
)

// The Hub's Docker collector refreshes containers once a minute, so their age
// runs up to about 62 s and crossed the 65 s agent limit between syncs. The
// "devices not responding" count jumped between 1 and 40. A connection's
// device must count only after it misses its own sync, and an agent device or
// a Docker container that really stopped must still count.
func TestCountStaleAssetsWaitsForAMissedConnectionSync(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	interval := func(seconds string) map[string]string {
		return map[string]string{assets.MetadataKeyCollectorInterval: seconds}
	}
	cases := []struct {
		name  string
		asset assets.Asset
		want  int
	}{
		{
			name: "docker container seen 70 s ago is between syncs",
			asset: assets.Asset{ID: "docker-ct-web", Type: "docker-container", Source: "docker",
				LastSeenAt: now.Add(-70 * time.Second), Metadata: interval("60")},
			want: 0,
		},
		{
			name: "agent host seen 70 s ago is not responding",
			asset: assets.Asset{ID: "host-nas01", Type: "host", Source: "agent",
				LastSeenAt: now.Add(-70 * time.Second)},
			want: 1,
		},
		{
			name: "docker container seen 10 min ago is not responding",
			asset: assets.Asset{ID: "docker-ct-db", Type: "docker-container", Source: "docker",
				LastSeenAt: now.Add(-10 * time.Minute), Metadata: interval("60")},
			want: 1,
		},
		{
			name: "proxmox guest of a 120 s connection seen 150 s ago is between syncs",
			asset: assets.Asset{ID: "proxmox-vm-101", Type: "vm", Source: "proxmox",
				LastSeenAt: now.Add(-150 * time.Second), Metadata: interval("120")},
			want: 0,
		},
	}
	all := make([]assets.Asset, 0, len(cases))
	wantTotal := 0
	for _, tc := range cases {
		if got := CountStaleAssets([]assets.Asset{tc.asset}, now); got != tc.want {
			t.Errorf("%s: counted %d, want %d", tc.name, got, tc.want)
		}
		all = append(all, tc.asset)
		wantTotal += tc.want
	}
	if got := CountStaleAssets(all, now); got != wantTotal {
		t.Errorf("all assets: counted %d, want %d", got, wantTotal)
	}
}

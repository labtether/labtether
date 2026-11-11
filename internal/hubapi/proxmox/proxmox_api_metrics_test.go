package proxmox

import (
	"net/url"
	"testing"
)

// The console's Metrics tab asks for its range as timeframe=; the Hub read only
// window=, so every range button showed the last hour.
func TestProxmoxMetricsTimeframeHonoursConsoleAndWindowParams(t *testing.T) {
	cases := map[string]string{
		"timeframe=day":            "day",
		"timeframe=week":           "week",
		"timeframe=month":          "month",
		"timeframe=Year":           "year",
		"window=24h":               "day",
		"window=30d":               "month",
		"timeframe=day&window=30d": "day",
		"timeframe=decade":         "hour",
		"":                         "hour",
	}
	for raw, want := range cases {
		query, err := url.ParseQuery(raw)
		if err != nil {
			t.Fatalf("parse %q: %v", raw, err)
		}
		if got := proxmoxMetricsTimeframe(query); got != want {
			t.Errorf("proxmoxMetricsTimeframe(%q) = %q, want %q", raw, got, want)
		}
	}
}

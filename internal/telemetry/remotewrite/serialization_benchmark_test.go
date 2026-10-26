package remotewrite

import (
	"fmt"
	"testing"
)

// These batches cover a normal scrape and backlog points sharing series. Keep
// both cases: reducing wire copies must not make repeated-series batches worse.
func BenchmarkSerializeWriteRequest(b *testing.B) {
	for _, seriesCount := range []int{10, 100, 500} {
		b.Run(fmt.Sprintf("series_%d", seriesCount), func(b *testing.B) {
			samples := make([]SampleWithLabels, 500)
			for i := range samples {
				samples[i] = SampleWithLabels{
					Labels: map[string]string{
						"__name__": "labtether_cpu_used_percent",
						"asset_id": fmt.Sprintf("host-%d", i%seriesCount),
						"platform": "linux", "group": "homelab",
					},
					Value: float64(i % 100), Timestamp: 1_800_000_000_000 + int64(i/seriesCount)*10_000,
				}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := SerializeWriteRequest(samples); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

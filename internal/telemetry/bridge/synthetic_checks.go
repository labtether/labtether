package bridge

import (
	"time"

	"github.com/labtether/labtether/internal/telemetry"
)

// SyntheticChecksSource provides synthetic check metrics.
type SyntheticChecksSource interface {
	// AllSyntheticCheckMetrics returns metrics for all known synthetic checks.
	AllSyntheticCheckMetrics() []SyntheticCheckEntry
}

// SyntheticCheckEntry holds per-check metric values and identifying labels.
type SyntheticCheckEntry struct {
	AssetID   string
	LatencyMs float64
	Status    float64           // 0=fail, 1=ok, 2=timeout
	Labels    map[string]string // check_type, target
}

// SyntheticChecksBridge is a MetricsBridge that reads synthetic check results
// from a SyntheticChecksSource and converts them to MetricSample objects.
type SyntheticChecksBridge struct {
	source SyntheticChecksSource
}

// NewSyntheticChecksBridge creates a SyntheticChecksBridge backed by the given source.
func NewSyntheticChecksBridge(source SyntheticChecksSource) *SyntheticChecksBridge {
	return &SyntheticChecksBridge{source: source}
}

// Name returns the bridge identifier.
func (b *SyntheticChecksBridge) Name() string { return "synthetic-checks" }

// Interval returns how often this bridge should be collected.
func (b *SyntheticChecksBridge) Interval() time.Duration { return 60 * time.Second }

// Collect iterates all synthetic checks from the source and produces 2 MetricSamples
// per check: synthetic_latency_ms and synthetic_status.
func (b *SyntheticChecksBridge) Collect() []telemetry.MetricSample {
	entries := b.source.AllSyntheticCheckMetrics()
	if len(entries) == 0 {
		return nil
	}

	now := time.Now().UTC()
	out := make([]telemetry.MetricSample, 0, len(entries)*2)

	for _, e := range entries {
		labels := e.Labels

		out = append(out,
			telemetry.MetricSample{
				AssetID:     e.AssetID,
				Metric:      telemetry.MetricSyntheticLatencyMs,
				Unit:        "ms",
				Value:       e.LatencyMs,
				CollectedAt: now,
				Labels:      labels,
			},
			telemetry.MetricSample{
				AssetID:     e.AssetID,
				Metric:      telemetry.MetricSyntheticStatus,
				Unit:        "status",
				Value:       e.Status,
				CollectedAt: now,
				Labels:      labels,
			},
		)
	}

	return out
}

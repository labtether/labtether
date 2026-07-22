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
	LatencyMs   float64
	HasLatency  bool
	Status      float64 // 0=fail, 1=ok, 2=timeout
	CollectedAt time.Time
	Labels      map[string]string // check_id, check_name, check_type; never target
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

	out := make([]telemetry.MetricSample, 0, len(entries)*2)

	for _, e := range entries {
		labels := e.Labels
		collectedAt := e.CollectedAt.UTC()
		if collectedAt.IsZero() {
			continue
		}

		if e.HasLatency {
			out = append(out, telemetry.MetricSample{
				Scope:       telemetry.MetricScopeHubSynthetic,
				Metric:      telemetry.MetricSyntheticLatencyMs,
				Unit:        "ms",
				Value:       e.LatencyMs,
				CollectedAt: collectedAt,
				Labels:      labels,
			})
		}
		out = append(out, telemetry.MetricSample{
			Scope:       telemetry.MetricScopeHubSynthetic,
			Metric:      telemetry.MetricSyntheticStatus,
			Unit:        "status",
			Value:       e.Status,
			CollectedAt: collectedAt,
			Labels:      labels,
		})
	}

	return out
}

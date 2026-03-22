package bridge

import (
	"time"

	"github.com/labtether/labtether/internal/telemetry"
)

// SiteReliabilitySource provides site reliability score metrics.
type SiteReliabilitySource interface {
	// AllSiteReliabilityMetrics returns reliability score entries for all sites.
	AllSiteReliabilityMetrics() []SiteReliabilityEntry
}

// SiteReliabilityEntry holds site-level reliability score and identifying labels.
// This is a hub-level metric (not per-asset); AssetID is fixed to "hub-reliability".
type SiteReliabilityEntry struct {
	Score  float64           // 0-100
	Labels map[string]string // site_id, site_name
}

// SiteReliabilityBridge is a MetricsBridge that reads site reliability scores
// from a SiteReliabilitySource and converts them to MetricSample objects.
type SiteReliabilityBridge struct {
	source SiteReliabilitySource
}

// NewSiteReliabilityBridge creates a SiteReliabilityBridge backed by the given source.
func NewSiteReliabilityBridge(source SiteReliabilitySource) *SiteReliabilityBridge {
	return &SiteReliabilityBridge{source: source}
}

// Name returns the bridge identifier.
func (b *SiteReliabilityBridge) Name() string { return "site-reliability" }

// Interval returns how often this bridge should be collected.
// Reliability scores are computed infrequently; 5 minutes is appropriate.
func (b *SiteReliabilityBridge) Interval() time.Duration { return 300 * time.Second }

// Collect iterates all site reliability entries from the source and produces
// 1 MetricSample per entry: site_reliability_score, keyed to the synthetic
// asset ID "hub-reliability".
func (b *SiteReliabilityBridge) Collect() []telemetry.MetricSample {
	entries := b.source.AllSiteReliabilityMetrics()
	if len(entries) == 0 {
		return nil
	}

	now := time.Now().UTC()
	out := make([]telemetry.MetricSample, 0, len(entries))

	for _, e := range entries {
		out = append(out, telemetry.MetricSample{
			AssetID:     "hub-reliability",
			Metric:      telemetry.MetricSiteReliabilityScore,
			Unit:        "score",
			Value:       e.Score,
			CollectedAt: now,
			Labels:      e.Labels,
		})
	}

	return out
}

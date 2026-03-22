package bridge

import (
	"time"

	"github.com/labtether/labtether/internal/telemetry"
)

// ProcessMetricsSource provides per-process CPU/memory metrics.
//
// Note: This bridge is opt-in (top-N processes by CPU). The bridge itself
// does not filter — the source implementation is expected to return only the
// top-N processes before passing them here.
type ProcessMetricsSource interface {
	// AllProcessMetrics returns metrics for the processes to be recorded.
	// The source is responsible for any top-N filtering by CPU usage.
	AllProcessMetrics() []ProcessMetricEntry
}

// ProcessMetricEntry holds per-process metric values and identifying labels.
type ProcessMetricEntry struct {
	AssetID    string
	CPUPercent float64
	MemPercent float64
	MemRSS     float64           // bytes
	Labels     map[string]string // process_name, process_pid
}

// ProcessMetricsBridge is a MetricsBridge that reads process metrics from a
// ProcessMetricsSource and converts them to MetricSample objects.
type ProcessMetricsBridge struct {
	source ProcessMetricsSource
}

// NewProcessMetricsBridge creates a ProcessMetricsBridge backed by the given source.
func NewProcessMetricsBridge(source ProcessMetricsSource) *ProcessMetricsBridge {
	return &ProcessMetricsBridge{source: source}
}

// Name returns the bridge identifier.
func (b *ProcessMetricsBridge) Name() string { return "process-metrics" }

// Interval returns how often this bridge should be collected.
func (b *ProcessMetricsBridge) Interval() time.Duration { return 30 * time.Second }

// Collect iterates all processes from the source and produces 3 MetricSamples
// per process: process_cpu_percent, process_memory_percent, and process_memory_rss_bytes.
func (b *ProcessMetricsBridge) Collect() []telemetry.MetricSample {
	entries := b.source.AllProcessMetrics()
	if len(entries) == 0 {
		return nil
	}

	now := time.Now().UTC()
	out := make([]telemetry.MetricSample, 0, len(entries)*3)

	for _, e := range entries {
		labels := e.Labels

		out = append(out,
			telemetry.MetricSample{
				AssetID:     e.AssetID,
				Metric:      telemetry.MetricProcessCPUPercent,
				Unit:        "percent",
				Value:       e.CPUPercent,
				CollectedAt: now,
				Labels:      labels,
			},
			telemetry.MetricSample{
				AssetID:     e.AssetID,
				Metric:      telemetry.MetricProcessMemoryPercent,
				Unit:        "percent",
				Value:       e.MemPercent,
				CollectedAt: now,
				Labels:      labels,
			},
			telemetry.MetricSample{
				AssetID:     e.AssetID,
				Metric:      telemetry.MetricProcessMemoryRSS,
				Unit:        "bytes",
				Value:       e.MemRSS,
				CollectedAt: now,
				Labels:      labels,
			},
		)
	}

	return out
}

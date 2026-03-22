package bridge

import (
	"time"

	"github.com/labtether/labtether/internal/telemetry"
)

// ProxmoxMetricsSource provides resource metrics from the Proxmox coordinator.
type ProxmoxMetricsSource interface {
	// AllProxmoxMetrics returns metrics for all known Proxmox nodes and VMs.
	AllProxmoxMetrics() []ProxmoxMetricEntry
}

// ProxmoxMetricEntry holds per-node/VM metric values and identifying labels.
type ProxmoxMetricEntry struct {
	AssetID    string
	CPU        float64 // 0–1 ratio; multiply by 100 for percent
	MemUsed    float64 // bytes
	MemTotal   float64 // bytes
	NetIn      float64 // bytes/sec
	NetOut     float64 // bytes/sec
	DiskRead   float64 // bytes/sec
	DiskWrite  float64 // bytes/sec
	Labels     map[string]string // proxmox_node
}

// ProxmoxMetricsBridge is a MetricsBridge that reads Proxmox resource metrics
// from a ProxmoxMetricsSource and converts them to MetricSample objects.
type ProxmoxMetricsBridge struct {
	source ProxmoxMetricsSource
}

// NewProxmoxMetricsBridge creates a ProxmoxMetricsBridge backed by the given source.
func NewProxmoxMetricsBridge(source ProxmoxMetricsSource) *ProxmoxMetricsBridge {
	return &ProxmoxMetricsBridge{source: source}
}

// Name returns the bridge identifier.
func (b *ProxmoxMetricsBridge) Name() string { return "proxmox-metrics" }

// Interval returns how often this bridge should be collected.
func (b *ProxmoxMetricsBridge) Interval() time.Duration { return 30 * time.Second }

// Collect iterates all Proxmox entries from the source and produces 7
// MetricSamples per entry: cpu, memory used/total, net in/out, disk read/write.
func (b *ProxmoxMetricsBridge) Collect() []telemetry.MetricSample {
	entries := b.source.AllProxmoxMetrics()
	if len(entries) == 0 {
		return nil
	}

	now := time.Now().UTC()
	out := make([]telemetry.MetricSample, 0, len(entries)*7)

	for _, e := range entries {
		labels := e.Labels

		out = append(out,
			telemetry.MetricSample{
				AssetID:     e.AssetID,
				Metric:      telemetry.MetricCPUUsedPercent,
				Unit:        "percent",
				Value:       e.CPU * 100,
				CollectedAt: now,
				Labels:      labels,
			},
			telemetry.MetricSample{
				AssetID:     e.AssetID,
				Metric:      telemetry.MetricMemoryUsedBytes,
				Unit:        "bytes",
				Value:       e.MemUsed,
				CollectedAt: now,
				Labels:      labels,
			},
			telemetry.MetricSample{
				AssetID:     e.AssetID,
				Metric:      telemetry.MetricMemoryTotalBytes,
				Unit:        "bytes",
				Value:       e.MemTotal,
				CollectedAt: now,
				Labels:      labels,
			},
			telemetry.MetricSample{
				AssetID:     e.AssetID,
				Metric:      telemetry.MetricNetworkRXBytesPerSec,
				Unit:        "bytes_per_sec",
				Value:       e.NetIn,
				CollectedAt: now,
				Labels:      labels,
			},
			telemetry.MetricSample{
				AssetID:     e.AssetID,
				Metric:      telemetry.MetricNetworkTXBytesPerSec,
				Unit:        "bytes_per_sec",
				Value:       e.NetOut,
				CollectedAt: now,
				Labels:      labels,
			},
			telemetry.MetricSample{
				AssetID:     e.AssetID,
				Metric:      telemetry.MetricDiskReadBytesPerSec,
				Unit:        "bytes_per_sec",
				Value:       e.DiskRead,
				CollectedAt: now,
				Labels:      labels,
			},
			telemetry.MetricSample{
				AssetID:     e.AssetID,
				Metric:      telemetry.MetricDiskWriteBytesPerSec,
				Unit:        "bytes_per_sec",
				Value:       e.DiskWrite,
				CollectedAt: now,
				Labels:      labels,
			},
		)
	}

	return out
}

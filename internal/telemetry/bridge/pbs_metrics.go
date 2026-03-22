package bridge

import (
	"time"

	"github.com/labtether/labtether/internal/telemetry"
)

// PBSMetricsSource provides datastore metrics from the Proxmox Backup Server coordinator.
type PBSMetricsSource interface {
	// AllPBSMetrics returns metrics for all known PBS datastores.
	AllPBSMetrics() []PBSMetricEntry
}

// PBSMetricEntry holds per-datastore metric values and identifying labels.
type PBSMetricEntry struct {
	AssetID     string
	Total       float64 // bytes
	Used        float64 // bytes
	Available   float64 // bytes
	BackupCount float64
	BackupAge   float64 // seconds since last backup
	GCPending   float64 // bytes
	Labels      map[string]string // datastore
}

// PBSMetricsBridge is a MetricsBridge that reads PBS datastore metrics from a
// PBSMetricsSource and converts them to MetricSample objects.
type PBSMetricsBridge struct {
	source PBSMetricsSource
}

// NewPBSMetricsBridge creates a PBSMetricsBridge backed by the given source.
func NewPBSMetricsBridge(source PBSMetricsSource) *PBSMetricsBridge {
	return &PBSMetricsBridge{source: source}
}

// Name returns the bridge identifier.
func (b *PBSMetricsBridge) Name() string { return "pbs-metrics" }

// Interval returns how often this bridge should be collected.
func (b *PBSMetricsBridge) Interval() time.Duration { return 60 * time.Second }

// Collect iterates all PBS entries from the source and produces 6
// MetricSamples per entry: storage total/used/available, backup count/age, gc pending.
func (b *PBSMetricsBridge) Collect() []telemetry.MetricSample {
	entries := b.source.AllPBSMetrics()
	if len(entries) == 0 {
		return nil
	}

	now := time.Now().UTC()
	out := make([]telemetry.MetricSample, 0, len(entries)*6)

	for _, e := range entries {
		labels := e.Labels

		out = append(out,
			telemetry.MetricSample{
				AssetID:     e.AssetID,
				Metric:      telemetry.MetricStorageTotalBytes,
				Unit:        "bytes",
				Value:       e.Total,
				CollectedAt: now,
				Labels:      labels,
			},
			telemetry.MetricSample{
				AssetID:     e.AssetID,
				Metric:      telemetry.MetricStorageUsedBytes,
				Unit:        "bytes",
				Value:       e.Used,
				CollectedAt: now,
				Labels:      labels,
			},
			telemetry.MetricSample{
				AssetID:     e.AssetID,
				Metric:      telemetry.MetricStorageAvailableBytes,
				Unit:        "bytes",
				Value:       e.Available,
				CollectedAt: now,
				Labels:      labels,
			},
			telemetry.MetricSample{
				AssetID:     e.AssetID,
				Metric:      telemetry.MetricBackupCount,
				Unit:        "count",
				Value:       e.BackupCount,
				CollectedAt: now,
				Labels:      labels,
			},
			telemetry.MetricSample{
				AssetID:     e.AssetID,
				Metric:      telemetry.MetricBackupAgeSeconds,
				Unit:        "seconds",
				Value:       e.BackupAge,
				CollectedAt: now,
				Labels:      labels,
			},
			telemetry.MetricSample{
				AssetID:     e.AssetID,
				Metric:      telemetry.MetricGCPendingBytes,
				Unit:        "bytes",
				Value:       e.GCPending,
				CollectedAt: now,
				Labels:      labels,
			},
		)
	}

	return out
}

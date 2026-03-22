package bridge

import (
	"time"

	"github.com/labtether/labtether/internal/telemetry"
)

// DiskMountsSource provides per-mount disk metrics from an asset.
type DiskMountsSource interface {
	// AllDiskMounts returns metrics for all known mounted filesystems.
	AllDiskMounts() []DiskMountEntry
}

// DiskMountEntry holds per-mount metric values and identifying labels.
type DiskMountEntry struct {
	AssetID   string
	Total     float64           // bytes
	Used      float64           // bytes
	Available float64           // bytes
	UsePct    float64           // percent
	Labels    map[string]string // mount_point
}

// DiskMountsBridge is a MetricsBridge that reads per-mount disk metrics from a
// DiskMountsSource and converts them to MetricSample objects.
type DiskMountsBridge struct {
	source DiskMountsSource
}

// NewDiskMountsBridge creates a DiskMountsBridge backed by the given source.
func NewDiskMountsBridge(source DiskMountsSource) *DiskMountsBridge {
	return &DiskMountsBridge{source: source}
}

// Name returns the bridge identifier.
func (b *DiskMountsBridge) Name() string { return "disk-mounts" }

// Interval returns how often this bridge should be collected.
func (b *DiskMountsBridge) Interval() time.Duration { return 60 * time.Second }

// Collect iterates all mounts from the source and produces 4 MetricSamples
// per mount: disk total/used/available bytes and used percent.
// Mounts are differentiated by the mount_point label.
func (b *DiskMountsBridge) Collect() []telemetry.MetricSample {
	entries := b.source.AllDiskMounts()
	if len(entries) == 0 {
		return nil
	}

	now := time.Now().UTC()
	out := make([]telemetry.MetricSample, 0, len(entries)*4)

	for _, e := range entries {
		labels := e.Labels

		out = append(out,
			telemetry.MetricSample{
				AssetID:     e.AssetID,
				Metric:      telemetry.MetricDiskTotalBytes,
				Unit:        "bytes",
				Value:       e.Total,
				CollectedAt: now,
				Labels:      labels,
			},
			telemetry.MetricSample{
				AssetID:     e.AssetID,
				Metric:      telemetry.MetricDiskUsedBytes,
				Unit:        "bytes",
				Value:       e.Used,
				CollectedAt: now,
				Labels:      labels,
			},
			telemetry.MetricSample{
				AssetID:     e.AssetID,
				Metric:      telemetry.MetricDiskAvailableBytes,
				Unit:        "bytes",
				Value:       e.Available,
				CollectedAt: now,
				Labels:      labels,
			},
			telemetry.MetricSample{
				AssetID:     e.AssetID,
				Metric:      telemetry.MetricDiskUsedPercent,
				Unit:        "percent",
				Value:       e.UsePct,
				CollectedAt: now,
				Labels:      labels,
			},
		)
	}

	return out
}

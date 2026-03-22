package bridge

import (
	"time"

	"github.com/labtether/labtether/internal/telemetry"
)

// DockerStatsSource provides container metrics from the Docker coordinator.
type DockerStatsSource interface {
	// AllContainerMetrics returns metrics for all known containers.
	AllContainerMetrics() []ContainerMetricEntry
}

// ContainerMetricEntry holds per-container metric values and identifying labels.
type ContainerMetricEntry struct {
	AssetID    string
	CPU        float64
	Memory     float64
	NetRX      float64 // bytes per sec
	NetTX      float64 // bytes per sec
	BlockRead  float64 // bytes per sec
	BlockWrite float64 // bytes per sec
	PIDs       float64
	Labels     map[string]string // docker_host, docker_image, docker_stack
}

// DockerStatsBridge is a MetricsBridge that reads container stats from a
// DockerStatsSource and converts them to MetricSample objects.
type DockerStatsBridge struct {
	source DockerStatsSource
}

// NewDockerStatsBridge creates a DockerStatsBridge backed by the given source.
func NewDockerStatsBridge(source DockerStatsSource) *DockerStatsBridge {
	return &DockerStatsBridge{source: source}
}

// Name returns the bridge identifier.
func (b *DockerStatsBridge) Name() string { return "docker-stats" }

// Interval returns how often this bridge should be collected.
func (b *DockerStatsBridge) Interval() time.Duration { return 15 * time.Second }

// Collect iterates all containers from the source and produces 7 MetricSamples
// per container: cpu, memory, net rx/tx, block read/write, and pids.
func (b *DockerStatsBridge) Collect() []telemetry.MetricSample {
	entries := b.source.AllContainerMetrics()
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
				Value:       e.CPU,
				CollectedAt: now,
				Labels:      labels,
			},
			telemetry.MetricSample{
				AssetID:     e.AssetID,
				Metric:      telemetry.MetricMemoryUsedPercent,
				Unit:        "percent",
				Value:       e.Memory,
				CollectedAt: now,
				Labels:      labels,
			},
			telemetry.MetricSample{
				AssetID:     e.AssetID,
				Metric:      telemetry.MetricNetworkRXBytesPerSec,
				Unit:        "bytes_per_sec",
				Value:       e.NetRX,
				CollectedAt: now,
				Labels:      labels,
			},
			telemetry.MetricSample{
				AssetID:     e.AssetID,
				Metric:      telemetry.MetricNetworkTXBytesPerSec,
				Unit:        "bytes_per_sec",
				Value:       e.NetTX,
				CollectedAt: now,
				Labels:      labels,
			},
			telemetry.MetricSample{
				AssetID:     e.AssetID,
				Metric:      telemetry.MetricBlockReadBytesPerSec,
				Unit:        "bytes_per_sec",
				Value:       e.BlockRead,
				CollectedAt: now,
				Labels:      labels,
			},
			telemetry.MetricSample{
				AssetID:     e.AssetID,
				Metric:      telemetry.MetricBlockWriteBytesPerSec,
				Unit:        "bytes_per_sec",
				Value:       e.BlockWrite,
				CollectedAt: now,
				Labels:      labels,
			},
			telemetry.MetricSample{
				AssetID:     e.AssetID,
				Metric:      telemetry.MetricPIDs,
				Unit:        "count",
				Value:       e.PIDs,
				CollectedAt: now,
				Labels:      labels,
			},
		)
	}

	return out
}

package telemetry

import (
	"strconv"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/metricschema"
)

const (
	MetricCPUUsedPercent       = metricschema.HeartbeatKeyCPUUsedPercent
	MetricMemoryUsedPercent    = metricschema.HeartbeatKeyMemoryUsedPercent
	MetricDiskUsedPercent      = metricschema.HeartbeatKeyDiskUsedPercent
	MetricTemperatureCelsius   = metricschema.HeartbeatKeyTemperatureCelsius
	MetricNetworkRXBytesPerSec = metricschema.HeartbeatKeyNetworkRXBytesPerSec
	MetricNetworkTXBytesPerSec = metricschema.HeartbeatKeyNetworkTXBytesPerSec

	// Container-specific
	MetricBlockReadBytesPerSec  = "block_read_bytes_per_sec"
	MetricBlockWriteBytesPerSec = "block_write_bytes_per_sec"
	MetricPIDs                  = "pids"

	// Per-disk
	MetricDiskTotalBytes     = "disk_total_bytes"
	MetricDiskUsedBytes      = "disk_used_bytes"
	MetricDiskAvailableBytes = "disk_available_bytes"

	// Per-interface
	MetricInterfaceRXBytesPerSec = "interface_rx_bytes_per_sec"
	MetricInterfaceTXBytesPerSec = "interface_tx_bytes_per_sec"
	MetricInterfaceRXPackets     = "interface_rx_packets"
	MetricInterfaceTXPackets     = "interface_tx_packets"

	// Proxmox
	MetricMemoryUsedBytes      = "memory_used_bytes"
	MetricMemoryTotalBytes     = "memory_total_bytes"
	MetricDiskReadBytesPerSec  = "disk_read_bytes_per_sec"
	MetricDiskWriteBytesPerSec = "disk_write_bytes_per_sec"

	// PBS
	MetricStorageTotalBytes     = "storage_total_bytes"
	MetricStorageUsedBytes      = "storage_used_bytes"
	MetricStorageAvailableBytes = "storage_available_bytes"
	MetricBackupCount           = "backup_count"
	MetricBackupAgeSeconds      = "backup_age_seconds"
	MetricGCPendingBytes        = "gc_pending_bytes"

	// Web service health
	MetricServiceResponseMs    = "service_response_ms"
	MetricServiceUptimePercent = "service_uptime_percent"
	MetricServiceStatus        = "service_status"

	// Synthetic checks
	MetricSyntheticLatencyMs = "synthetic_latency_ms"
	MetricSyntheticStatus    = "synthetic_status"

	// Site reliability
	MetricSiteReliabilityScore = "site_reliability_score"

	// Alert state
	MetricAlertsFiring              = "alerts_firing"
	MetricAlertsRules               = "alerts_rules"
	MetricAlertEvaluationDurationMs = "alert_evaluation_duration_ms"

	// Agent presence
	MetricAgentConnected               = "agent_connected"
	MetricAgentLastHeartbeatAgeSeconds = "agent_last_heartbeat_age_seconds"

	// Process-level
	MetricProcessCPUPercent    = "process_cpu_percent"
	MetricProcessMemoryPercent = "process_memory_percent"
	MetricProcessMemoryRSS     = "process_memory_rss_bytes"
)

// MetricDefinition describes a canonical telemetry metric.
type MetricDefinition struct {
	Metric string
	Unit   string
}

var canonicalMetrics = []MetricDefinition{
	{Metric: MetricCPUUsedPercent, Unit: "percent"},
	{Metric: MetricMemoryUsedPercent, Unit: "percent"},
	{Metric: MetricDiskUsedPercent, Unit: "percent"},
	{Metric: MetricTemperatureCelsius, Unit: "celsius"},
	{Metric: MetricNetworkRXBytesPerSec, Unit: "bytes_per_sec"},
	{Metric: MetricNetworkTXBytesPerSec, Unit: "bytes_per_sec"},
}

// CanonicalMetrics returns the ordered canonical metrics supported by LabTether.
func CanonicalMetrics() []MetricDefinition {
	out := make([]MetricDefinition, len(canonicalMetrics))
	copy(out, canonicalMetrics)
	return out
}

// MetricSample is a single normalized telemetry sample point.
type MetricSample struct {
	AssetID     string
	Metric      string
	Unit        string
	Value       float64
	CollectedAt time.Time
	// Labels carries optional sub-asset dimensions (e.g. mount_point, interface, container).
	// Nil/empty means no labels; persisted as NULL in metric_samples.labels.
	Labels map[string]string
}

// Snapshot contains latest values per canonical metric for an asset.
type Snapshot struct {
	CPUUsedPercent       *float64 `json:"cpu_used_percent,omitempty"`
	MemoryUsedPercent    *float64 `json:"memory_used_percent,omitempty"`
	DiskUsedPercent      *float64 `json:"disk_used_percent,omitempty"`
	TemperatureCelsius   *float64 `json:"temperature_celsius,omitempty"`
	NetworkRXBytesPerSec *float64 `json:"network_rx_bytes_per_sec,omitempty"`
	NetworkTXBytesPerSec *float64 `json:"network_tx_bytes_per_sec,omitempty"`
}

// DynamicSnapshot holds the latest value for each metric on an asset.
// Unlike Snapshot (which has 6 fixed fields), DynamicSnapshot supports any metric.
type DynamicSnapshot struct {
	Metrics map[string]float64
}

// ToLegacySnapshot converts a DynamicSnapshot to the fixed-field Snapshot for
// backward-compatible API responses.
func (d DynamicSnapshot) ToLegacySnapshot() Snapshot {
	snap := Snapshot{}
	if v, ok := d.Metrics[MetricCPUUsedPercent]; ok {
		snap.CPUUsedPercent = &v
	}
	if v, ok := d.Metrics[MetricMemoryUsedPercent]; ok {
		snap.MemoryUsedPercent = &v
	}
	if v, ok := d.Metrics[MetricDiskUsedPercent]; ok {
		snap.DiskUsedPercent = &v
	}
	if v, ok := d.Metrics[MetricTemperatureCelsius]; ok {
		snap.TemperatureCelsius = &v
	}
	if v, ok := d.Metrics[MetricNetworkRXBytesPerSec]; ok {
		snap.NetworkRXBytesPerSec = &v
	}
	if v, ok := d.Metrics[MetricNetworkTXBytesPerSec]; ok {
		snap.NetworkTXBytesPerSec = &v
	}
	return snap
}

// Series is a canonical metric time series for an asset.
type Series struct {
	Metric  string   `json:"metric"`
	Unit    string   `json:"unit"`
	Points  []Point  `json:"points"`
	Current *float64 `json:"current,omitempty"`
}

// SamplesFromHeartbeatMetadata converts heartbeat metadata values into canonical metric samples.
func SamplesFromHeartbeatMetadata(assetID string, collectedAt time.Time, metadata map[string]string) []MetricSample {
	if assetID == "" || len(metadata) == 0 {
		return nil
	}

	keyToMetric := []struct {
		Keys   []string
		Metric string
		Unit   string
	}{
		{
			Keys:   []string{metricschema.HeartbeatKeyCPUPercent, metricschema.HeartbeatKeyCPUUsedPercent},
			Metric: MetricCPUUsedPercent,
			Unit:   "percent",
		},
		{
			Keys:   []string{metricschema.HeartbeatKeyMemoryPercent, metricschema.HeartbeatKeyMemoryUsedPercent},
			Metric: MetricMemoryUsedPercent,
			Unit:   "percent",
		},
		{
			Keys:   []string{metricschema.HeartbeatKeyDiskPercent, metricschema.HeartbeatKeyDiskUsedPercent},
			Metric: MetricDiskUsedPercent,
			Unit:   "percent",
		},
		{
			Keys:   []string{metricschema.HeartbeatKeyTempCelsius, metricschema.HeartbeatKeyTemperatureCelsius},
			Metric: MetricTemperatureCelsius,
			Unit:   "celsius",
		},
		{
			Keys:   []string{metricschema.HeartbeatKeyNetworkRXBytesPerSec},
			Metric: MetricNetworkRXBytesPerSec,
			Unit:   "bytes_per_sec",
		},
		{
			Keys:   []string{metricschema.HeartbeatKeyNetworkTXBytesPerSec},
			Metric: MetricNetworkTXBytesPerSec,
			Unit:   "bytes_per_sec",
		},
	}

	out := make([]MetricSample, 0, len(keyToMetric))
	for _, mapping := range keyToMetric {
		raw, ok := firstMetadataValue(metadata, mapping.Keys...)
		if !ok {
			continue
		}

		value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
		if err != nil {
			continue
		}
		out = append(out, MetricSample{
			AssetID:     assetID,
			Metric:      mapping.Metric,
			Unit:        mapping.Unit,
			Value:       value,
			CollectedAt: collectedAt.UTC(),
		})
	}

	return out
}

// BuildDirectSamples creates metric samples from pre-parsed numeric values,
// used by the WebSocket agent transport where values arrive as typed fields
// rather than string metadata.
func BuildDirectSamples(assetID string, collectedAt time.Time,
	cpu, mem, disk, netRX, netTX float64, temp *float64,
) []MetricSample {
	if assetID == "" {
		return nil
	}
	out := []MetricSample{
		{AssetID: assetID, Metric: MetricCPUUsedPercent, Unit: "percent", Value: cpu, CollectedAt: collectedAt},
		{AssetID: assetID, Metric: MetricMemoryUsedPercent, Unit: "percent", Value: mem, CollectedAt: collectedAt},
		{AssetID: assetID, Metric: MetricDiskUsedPercent, Unit: "percent", Value: disk, CollectedAt: collectedAt},
		{AssetID: assetID, Metric: MetricNetworkRXBytesPerSec, Unit: "bytes_per_sec", Value: netRX, CollectedAt: collectedAt},
		{AssetID: assetID, Metric: MetricNetworkTXBytesPerSec, Unit: "bytes_per_sec", Value: netTX, CollectedAt: collectedAt},
	}
	if temp != nil {
		out = append(out, MetricSample{AssetID: assetID, Metric: MetricTemperatureCelsius, Unit: "celsius", Value: *temp, CollectedAt: collectedAt})
	}
	return out
}

func firstMetadataValue(metadata map[string]string, keys ...string) (string, bool) {
	for _, key := range keys {
		if value, ok := metadata[key]; ok {
			return value, true
		}
	}
	return "", false
}

// LastPointValue returns the latest value in a point list.
func LastPointValue(points []Point) *float64 {
	if len(points) == 0 {
		return nil
	}
	value := points[len(points)-1].Value
	return &value
}

// BucketAveragePoints downsamples points by averaging values in each step bucket.
func BucketAveragePoints(points []Point, step time.Duration) []Point {
	if len(points) == 0 || step <= 0 {
		return points
	}

	seconds := int64(step.Seconds())
	if seconds <= 1 {
		return points
	}

	out := make([]Point, 0, len(points))
	currentBucket := int64(0)
	var sum float64
	count := 0
	first := true

	flush := func() {
		if count == 0 {
			return
		}
		out = append(out, Point{
			TS:    currentBucket,
			Value: sum / float64(count),
		})
		sum = 0
		count = 0
	}

	for _, point := range points {
		bucket := (point.TS / seconds) * seconds
		if first {
			currentBucket = bucket
			first = false
		}
		if bucket != currentBucket {
			flush()
			currentBucket = bucket
		}
		sum += point.Value
		count++
	}

	flush()
	return out
}

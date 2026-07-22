package telemetry

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

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

// Hub metric scopes are non-asset telemetry namespaces. They are persisted
// separately from asset metrics so global hub gauges do not need fake rows in
// the assets table and cannot violate the metric_samples asset foreign key.
const (
	MetricScopeHubAlerts      = "hub-alerts"
	MetricScopeHubReliability = "hub-reliability"
	MetricScopeHubSynthetic   = "hub-synthetic"

	// HubMetricSnapshotMaxAge bounds each Prometheus snapshot query and
	// expires labeled series that a bridge no longer emits (for example, a
	// deleted alert rule). It covers at least three reliability collection
	// intervals and many alert-state intervals.
	HubMetricSnapshotMaxAge = 15 * time.Minute

	// MaxHubMetricSeriesPerScope bounds labeled cardinality for each global
	// scope. MaxHubMetricSnapshotSeries covers all currently supported scopes.
	MaxHubMetricSeriesPerScope     = 1024
	MaxHubMetricSnapshotSeries     = 3 * MaxHubMetricSeriesPerScope
	MaxHubMetricHistoryPerSeries   = 16
	MaxSiteReliabilityMetricSeries = MaxHubMetricSeriesPerScope

	// MaxAlertRuleMetricSeries bounds per-rule evaluation gauges while the
	// aggregate active-rule count remains exact.
	MaxAlertRuleMetricSeries = 500

	// Prometheus asset snapshots are explicitly bounded in age, requested
	// assets, raw rows examined, and exported labeled series. These limits keep
	// a scrape from turning into an unbounded telemetry-history query.
	PrometheusAssetSnapshotMaxAge  = 15 * time.Minute
	MaxPrometheusSnapshotAssets    = 5000
	MaxPrometheusAssetMetricRows   = 100000
	MaxPrometheusAssetMetricSeries = 10000

	// Bridge source limits cap agent- and connector-controlled cardinality
	// before samples reach persistence. Agent inventory collection rotates
	// through larger fleets rather than polling every connected endpoint at
	// once.
	MaxBridgeAgentAssets           = 64
	MaxBridgeProcessesPerAsset     = 200
	MaxBridgeInterfacesPerAsset    = 128
	MaxBridgeMountsPerAsset        = 128
	MaxBridgeServiceSeries         = 2048
	MaxBridgeSyntheticSeries       = 500
	MaxBridgeDockerContainerSeries = 5000
	MaxBridgeAgentPresenceSeries   = 5000
	MaxBridgePBSDataStoreSeries    = 2000

	// Metric ingestion bounds protect both in-memory storage and PostgreSQL
	// statement construction from agent-controlled names, labels, and batches.
	MaxMetricNameBytes        = 256
	MaxMetricUnitBytes        = 64
	MaxMetricIdentityBytes    = 1024
	MaxMetricLabelsPerSample  = 64
	MaxMetricLabelKeyBytes    = 128
	MaxMetricLabelValueBytes  = 1024
	MaxMetricLabelsBytes      = 8 * 1024
	MaxMetricSampleBytes      = 16 * 1024
	MaxMetricSamplesPerAppend = 50000
	MaxMetricAppendBytes      = 64 * 1024 * 1024
	MaxPrometheusExportBytes  = 8 * 1024 * 1024
)

// MetricSampleEnvelopeBytes validates bounded, UTF-8-safe sample-controlled
// strings and returns a conservative payload-byte estimate for aggregate batch
// budgeting. Semantic metric/scope validation remains the store's job.
func MetricSampleEnvelopeBytes(sample MetricSample) (int, error) {
	if math.IsNaN(sample.Value) || math.IsInf(sample.Value, 0) {
		return 0, fmt.Errorf("metric sample value must be finite")
	}
	fields := []struct {
		name  string
		value string
		max   int
	}{
		{name: "asset_id", value: sample.AssetID, max: MaxMetricIdentityBytes},
		{name: "scope", value: sample.Scope, max: MaxMetricIdentityBytes},
		{name: "metric", value: sample.Metric, max: MaxMetricNameBytes},
		{name: "unit", value: sample.Unit, max: MaxMetricUnitBytes},
	}
	total := 32 // numeric value and timestamp overhead
	for _, field := range fields {
		if len(field.value) > field.max || !utf8.ValidString(field.value) || strings.ContainsRune(field.value, '\x00') {
			return 0, fmt.Errorf("metric sample %s exceeds UTF-8/byte limit", field.name)
		}
		total += len(field.value)
	}
	if len(sample.Labels) > MaxMetricLabelsPerSample {
		return 0, fmt.Errorf("metric sample label count exceeds limit")
	}
	for key, value := range sample.Labels {
		if len(key) == 0 || len(key) > MaxMetricLabelKeyBytes || !utf8.ValidString(key) || strings.ContainsRune(key, '\x00') {
			return 0, fmt.Errorf("metric sample label key exceeds UTF-8/byte limit")
		}
		if len(value) > MaxMetricLabelValueBytes || !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') {
			return 0, fmt.Errorf("metric sample label value exceeds UTF-8/byte limit")
		}
	}
	labelBytes := 0
	if len(sample.Labels) > 0 {
		encoded, err := json.Marshal(sample.Labels)
		if err != nil {
			return 0, fmt.Errorf("marshal metric sample labels: %w", err)
		}
		labelBytes = len(encoded)
		if labelBytes > MaxMetricLabelsBytes {
			return 0, fmt.Errorf("metric sample encoded label bytes exceed limit")
		}
	}
	total += labelBytes
	if total > MaxMetricSampleBytes {
		return 0, fmt.Errorf("metric sample bytes exceed limit")
	}
	return total, nil
}

// IsHubMetricScope reports whether scope is a supported non-asset telemetry
// namespace. Keeping the set explicit prevents arbitrary unbounded namespaces
// from being written by a malformed producer.
func IsHubMetricScope(scope string) bool {
	switch strings.TrimSpace(scope) {
	case MetricScopeHubAlerts, MetricScopeHubReliability, MetricScopeHubSynthetic:
		return true
	default:
		return false
	}
}

// HubMetricLabelKeys returns the exact ordered label schema for a supported
// hub metric. Hub metrics deliberately use a closed schema: accepting an
// arbitrary metric or label here could create duplicate Prometheus series or
// incompatible descriptors for one exported metric name.
func HubMetricLabelKeys(scope, metric string) ([]string, bool) {
	switch strings.TrimSpace(scope) {
	case MetricScopeHubAlerts:
		switch strings.TrimSpace(metric) {
		case MetricAlertsFiring, MetricAlertsRules:
			return nil, true
		case MetricAlertEvaluationDurationMs:
			return []string{"rule_id", "rule_name"}, true
		}
	case MetricScopeHubReliability:
		if strings.TrimSpace(metric) == MetricSiteReliabilityScore {
			return []string{"site_id", "site_name"}, true
		}
	case MetricScopeHubSynthetic:
		switch strings.TrimSpace(metric) {
		case MetricSyntheticLatencyMs, MetricSyntheticStatus:
			return []string{"check_id", "check_name", "check_type"}, true
		}
	}
	return nil, false
}

// NormalizeHubMetricLabels validates a hub metric's scope/metric pairing and
// exact label schema, returning a copy with canonical nil handling. Stable ID
// labels must be non-empty; display names may be blank without dropping the
// aggregate counts or otherwise healthy series in the same collection.
func NormalizeHubMetricLabels(scope, metric string, labels map[string]string) (map[string]string, error) {
	keys, ok := HubMetricLabelKeys(scope, metric)
	if !ok {
		return nil, fmt.Errorf("unsupported hub metric %q in scope %q", strings.TrimSpace(metric), strings.TrimSpace(scope))
	}
	if len(keys) == 0 {
		if len(labels) != 0 {
			return nil, fmt.Errorf("hub metric %q in scope %q does not accept labels", strings.TrimSpace(metric), strings.TrimSpace(scope))
		}
		return nil, nil
	}
	if len(labels) != len(keys) {
		return nil, fmt.Errorf("hub metric %q in scope %q requires labels %v", strings.TrimSpace(metric), strings.TrimSpace(scope), keys)
	}
	canonical := make(map[string]string, len(keys))
	for _, key := range keys {
		value, exists := labels[key]
		value = strings.TrimSpace(value)
		if !exists {
			return nil, fmt.Errorf("hub metric %q in scope %q requires label %q", strings.TrimSpace(metric), strings.TrimSpace(scope), key)
		}
		if strings.HasSuffix(key, "_id") && value == "" {
			return nil, fmt.Errorf("hub metric %q in scope %q requires non-empty identity label %q", strings.TrimSpace(metric), strings.TrimSpace(scope), key)
		}
		canonical[key] = value
	}
	return canonical, nil
}

// NormalizeHubMetricSample validates and canonicalizes one non-asset sample.
func NormalizeHubMetricSample(sample MetricSample) (MetricSample, error) {
	sample.Scope = strings.TrimSpace(sample.Scope)
	sample.Metric = strings.TrimSpace(sample.Metric)
	sample.Unit = strings.TrimSpace(sample.Unit)
	if sample.Scope == "" || sample.AssetID != "" {
		return MetricSample{}, fmt.Errorf("hub metric sample must set scope and must not set asset_id")
	}
	labels, err := NormalizeHubMetricLabels(sample.Scope, sample.Metric, sample.Labels)
	if err != nil {
		return MetricSample{}, err
	}
	expectedUnit := ""
	switch sample.Metric {
	case MetricAlertsFiring, MetricAlertsRules:
		expectedUnit = "count"
	case MetricAlertEvaluationDurationMs:
		expectedUnit = "ms"
	case MetricSiteReliabilityScore:
		expectedUnit = "score"
	case MetricSyntheticLatencyMs:
		expectedUnit = "ms"
	case MetricSyntheticStatus:
		expectedUnit = "status"
	}
	if sample.Unit != expectedUnit {
		return MetricSample{}, fmt.Errorf("hub metric %q requires unit %q", sample.Metric, expectedUnit)
	}
	sample.Labels = labels
	return sample, nil
}

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
	AssetID string
	// Scope identifies a supported non-asset telemetry namespace. Exactly one
	// of AssetID or Scope must be set for a persisted sample.
	Scope       string
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
		if !isFiniteMetricValue(value) {
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
	out := make([]MetricSample, 0, 6)
	out = appendMetricSampleIfFinite(out, MetricSample{AssetID: assetID, Metric: MetricCPUUsedPercent, Unit: "percent", Value: cpu, CollectedAt: collectedAt})
	out = appendMetricSampleIfFinite(out, MetricSample{AssetID: assetID, Metric: MetricMemoryUsedPercent, Unit: "percent", Value: mem, CollectedAt: collectedAt})
	out = appendMetricSampleIfFinite(out, MetricSample{AssetID: assetID, Metric: MetricDiskUsedPercent, Unit: "percent", Value: disk, CollectedAt: collectedAt})
	out = appendMetricSampleIfFinite(out, MetricSample{AssetID: assetID, Metric: MetricNetworkRXBytesPerSec, Unit: "bytes_per_sec", Value: netRX, CollectedAt: collectedAt})
	out = appendMetricSampleIfFinite(out, MetricSample{AssetID: assetID, Metric: MetricNetworkTXBytesPerSec, Unit: "bytes_per_sec", Value: netTX, CollectedAt: collectedAt})
	if temp != nil {
		out = appendMetricSampleIfFinite(out, MetricSample{AssetID: assetID, Metric: MetricTemperatureCelsius, Unit: "celsius", Value: *temp, CollectedAt: collectedAt})
	}
	return out
}

func appendMetricSampleIfFinite(out []MetricSample, sample MetricSample) []MetricSample {
	if !isFiniteMetricValue(sample.Value) {
		return out
	}
	return append(out, sample)
}

func isFiniteMetricValue(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
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

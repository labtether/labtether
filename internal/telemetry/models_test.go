package telemetry

import (
	"math"
	"strings"
	"testing"
	"time"
)

func TestMetricSample_Labels(t *testing.T) {
	now := time.Now().UTC()

	// Zero value: Labels is nil by default.
	s := MetricSample{
		AssetID:     "asset-1",
		Metric:      MetricDiskUsedPercent,
		Unit:        "percent",
		Value:       72.5,
		CollectedAt: now,
	}
	if s.Labels != nil {
		t.Errorf("expected nil Labels for zero-value sample, got %v", s.Labels)
	}

	// Populated labels round-trip.
	s.Labels = map[string]string{
		"mount_point": "/data",
		"device":      "sdb1",
	}
	if got := s.Labels["mount_point"]; got != "/data" {
		t.Errorf("Labels[mount_point] = %q, want %q", got, "/data")
	}
	if got := s.Labels["device"]; got != "sdb1" {
		t.Errorf("Labels[device] = %q, want %q", got, "sdb1")
	}

	// Labels from a separate sample must not alias.
	s2 := MetricSample{
		AssetID:     "asset-2",
		Metric:      MetricNetworkRXBytesPerSec,
		Unit:        "bytes_per_sec",
		Value:       1024,
		CollectedAt: now,
		Labels:      map[string]string{"interface": "eth0"},
	}
	if _, ok := s2.Labels["mount_point"]; ok {
		t.Error("s2.Labels must not contain keys from s.Labels")
	}
	if got := s2.Labels["interface"]; got != "eth0" {
		t.Errorf("s2.Labels[interface] = %q, want %q", got, "eth0")
	}
}

func TestMetricSampleEnvelopeBytesRejectsUnboundedOrNonFiniteInput(t *testing.T) {
	valid := MetricSample{
		AssetID: "asset-1", Metric: MetricDiskUsedBytes, Unit: "bytes", Value: 1,
		Labels: map[string]string{"mount_point": "/data"},
	}
	if size, err := MetricSampleEnvelopeBytes(valid); err != nil || size <= 0 || size > MaxMetricSampleBytes {
		t.Fatalf("valid envelope size=%d err=%v", size, err)
	}

	cases := []MetricSample{
		{AssetID: "asset-1", Metric: MetricCPUUsedPercent, Unit: "percent", Value: math.NaN()},
		{AssetID: "asset-1", Metric: MetricCPUUsedPercent, Unit: "percent", Value: math.Inf(1)},
		{AssetID: "asset-1", Metric: strings.Repeat("m", MaxMetricNameBytes+1), Unit: "count", Value: 1},
		{AssetID: "asset-1", Metric: "metric", Unit: strings.Repeat("u", MaxMetricUnitBytes+1), Value: 1},
		{AssetID: "asset-1", Metric: "metric", Unit: "count", Value: 1, Labels: map[string]string{strings.Repeat("k", MaxMetricLabelKeyBytes+1): "v"}},
		{AssetID: "asset-1", Metric: "metric", Unit: "count", Value: 1, Labels: map[string]string{"key": strings.Repeat("v", MaxMetricLabelValueBytes+1)}},
		{AssetID: "asset\x00one", Metric: "metric", Unit: "count", Value: 1},
		{AssetID: "asset-1", Metric: "metric\x00name", Unit: "count", Value: 1},
		{AssetID: "asset-1", Metric: "metric", Unit: "count\x00unit", Value: 1},
		{AssetID: "asset-1", Metric: "metric", Unit: "count", Value: 1, Labels: map[string]string{"bad\x00key": "v"}},
		{AssetID: "asset-1", Metric: "metric", Unit: "count", Value: 1, Labels: map[string]string{"key": "bad\x00value"}},
	}
	for i, sample := range cases {
		if _, err := MetricSampleEnvelopeBytes(sample); err == nil {
			t.Fatalf("invalid envelope case %d was accepted", i)
		}
	}
}

func TestMetricSample_LabelsEmpty(t *testing.T) {
	// An empty (non-nil) map should be valid.
	s := MetricSample{
		AssetID:     "asset-3",
		Metric:      MetricCPUUsedPercent,
		Unit:        "percent",
		Value:       50.0,
		CollectedAt: time.Now().UTC(),
		Labels:      map[string]string{},
	}
	if s.Labels == nil {
		t.Error("expected non-nil empty Labels map")
	}
	if len(s.Labels) != 0 {
		t.Errorf("expected empty Labels, got len=%d", len(s.Labels))
	}
}

func TestIsHubMetricScopeIsExplicit(t *testing.T) {
	for _, scope := range []string{MetricScopeHubAlerts, MetricScopeHubReliability, MetricScopeHubSynthetic} {
		if !IsHubMetricScope(scope) {
			t.Fatalf("expected supported hub metric scope %q", scope)
		}
	}
	for _, scope := range []string{"", "hub-arbitrary", "asset-1"} {
		if IsHubMetricScope(scope) {
			t.Fatalf("unexpected supported hub metric scope %q", scope)
		}
	}
}

func TestNormalizeSyntheticHubMetricUsesClosedNonTargetSchema(t *testing.T) {
	sample := MetricSample{
		Scope: MetricScopeHubSynthetic, Metric: MetricSyntheticLatencyMs, Unit: "ms", Value: 12,
		CollectedAt: time.Now().UTC(),
		Labels:      map[string]string{"check_id": "check-1", "check_name": "Private endpoint", "check_type": "http"},
	}
	if _, err := NormalizeHubMetricSample(sample); err != nil {
		t.Fatalf("valid synthetic hub sample rejected: %v", err)
	}
	sample.Labels["target"] = "https://user:password@example.test/?token=secret"
	if _, err := NormalizeHubMetricSample(sample); err == nil {
		t.Fatal("synthetic target label was accepted outside the closed schema")
	}
}

func TestSamplesFromHeartbeatMetadataSkipsNonFiniteValues(t *testing.T) {
	now := time.Now().UTC()
	samples := SamplesFromHeartbeatMetadata("asset-1", now, map[string]string{
		MetricCPUUsedPercent:       "42",
		MetricMemoryUsedPercent:    "NaN",
		MetricDiskUsedPercent:      "+Inf",
		MetricTemperatureCelsius:   "-Inf",
		MetricNetworkRXBytesPerSec: "512",
	})

	byMetric := make(map[string]MetricSample, len(samples))
	for _, sample := range samples {
		byMetric[sample.Metric] = sample
	}

	if got := byMetric[MetricCPUUsedPercent].Value; got != 42 {
		t.Fatalf("cpu sample = %v, want 42", got)
	}
	if got := byMetric[MetricNetworkRXBytesPerSec].Value; got != 512 {
		t.Fatalf("rx sample = %v, want 512", got)
	}
	for _, metric := range []string{MetricMemoryUsedPercent, MetricDiskUsedPercent, MetricTemperatureCelsius} {
		if _, ok := byMetric[metric]; ok {
			t.Fatalf("unexpected non-finite sample for %s", metric)
		}
	}
}

func TestBuildDirectSamplesSkipsNonFiniteValues(t *testing.T) {
	temp := math.Inf(1)
	samples := BuildDirectSamples("asset-1", time.Now().UTC(), 15, math.NaN(), math.Inf(-1), 1024, math.Inf(1), &temp)

	byMetric := make(map[string]MetricSample, len(samples))
	for _, sample := range samples {
		byMetric[sample.Metric] = sample
	}

	if got := byMetric[MetricCPUUsedPercent].Value; got != 15 {
		t.Fatalf("cpu sample = %v, want 15", got)
	}
	if got := byMetric[MetricNetworkRXBytesPerSec].Value; got != 1024 {
		t.Fatalf("rx sample = %v, want 1024", got)
	}
	for _, metric := range []string{MetricMemoryUsedPercent, MetricDiskUsedPercent, MetricNetworkTXBytesPerSec, MetricTemperatureCelsius} {
		if _, ok := byMetric[metric]; ok {
			t.Fatalf("unexpected non-finite sample for %s", metric)
		}
	}
}

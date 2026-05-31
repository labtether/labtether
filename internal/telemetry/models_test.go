package telemetry

import (
	"math"
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

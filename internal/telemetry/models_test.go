package telemetry

import (
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

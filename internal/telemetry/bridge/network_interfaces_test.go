package bridge

import (
	"testing"

	"github.com/labtether/labtether/internal/telemetry"
)

// mockNetworkInterfacesSource implements NetworkInterfacesSource for testing.
type mockNetworkInterfacesSource struct {
	entries []NetworkInterfaceEntry
}

func (m *mockNetworkInterfacesSource) AllNetworkInterfaces() []NetworkInterfaceEntry {
	return m.entries
}

func TestNetworkInterfacesBridgeCollect(t *testing.T) {
	source := &mockNetworkInterfacesSource{
		entries: []NetworkInterfaceEntry{
			{
				AssetID:   "linux-asset-server01",
				RXBytes:   125000000, // ~1 Gbps
				TXBytes:   62500000,
				RXPackets: 90000,
				TXPackets: 45000,
				Labels: map[string]string{
					"interface": "eth0",
				},
			},
			{
				AssetID:   "linux-asset-server01",
				RXBytes:   1024,
				TXBytes:   512,
				RXPackets: 10,
				TXPackets: 5,
				Labels: map[string]string{
					"interface": "lo",
				},
			},
		},
	}

	b := NewNetworkInterfacesBridge(source)

	if b.Name() != "network-interfaces" {
		t.Errorf("unexpected Name: %q", b.Name())
	}

	samples := b.Collect()

	if len(samples) != 8 {
		t.Fatalf("expected 8 samples (4 per interface), got %d", len(samples))
	}

	// Build lookup: "assetID:metric:interface" -> sample to distinguish interfaces on same asset.
	byKey := make(map[string]telemetry.MetricSample, len(samples))
	for _, s := range samples {
		iface := s.Labels["interface"]
		byKey[s.AssetID+":"+s.Metric+":"+iface] = s
	}

	assetID := "linux-asset-server01"

	// eth0
	assertSampleByIface(t, byKey, assetID, telemetry.MetricInterfaceRXBytesPerSec, "bytes_per_sec", 125000000, "eth0")
	assertSampleByIface(t, byKey, assetID, telemetry.MetricInterfaceTXBytesPerSec, "bytes_per_sec", 62500000, "eth0")
	assertSampleByIface(t, byKey, assetID, telemetry.MetricInterfaceRXPackets, "count", 90000, "eth0")
	assertSampleByIface(t, byKey, assetID, telemetry.MetricInterfaceTXPackets, "count", 45000, "eth0")

	// lo
	assertSampleByIface(t, byKey, assetID, telemetry.MetricInterfaceRXBytesPerSec, "bytes_per_sec", 1024, "lo")
	assertSampleByIface(t, byKey, assetID, telemetry.MetricInterfaceTXBytesPerSec, "bytes_per_sec", 512, "lo")
	assertSampleByIface(t, byKey, assetID, telemetry.MetricInterfaceRXPackets, "count", 10, "lo")
	assertSampleByIface(t, byKey, assetID, telemetry.MetricInterfaceTXPackets, "count", 5, "lo")
}

func TestNetworkInterfacesBridgeEmpty(t *testing.T) {
	source := &mockNetworkInterfacesSource{entries: nil}
	b := NewNetworkInterfacesBridge(source)

	samples := b.Collect()
	if len(samples) != 0 {
		t.Fatalf("expected 0 samples from empty source, got %d", len(samples))
	}
}

func TestNetworkInterfacesBridgeLabelsHaveInterfaceName(t *testing.T) {
	wantIface := "bond0"

	source := &mockNetworkInterfacesSource{
		entries: []NetworkInterfaceEntry{
			{
				AssetID:   "linux-asset-server03",
				RXBytes:   500000,
				TXBytes:   250000,
				RXPackets: 500,
				TXPackets: 250,
				Labels: map[string]string{
					"interface": wantIface,
				},
			},
		},
	}

	b := NewNetworkInterfacesBridge(source)
	samples := b.Collect()

	if len(samples) != 4 {
		t.Fatalf("expected 4 samples, got %d", len(samples))
	}

	for _, s := range samples {
		got, ok := s.Labels["interface"]
		if !ok {
			t.Errorf("metric %q: missing interface label", s.Metric)
			continue
		}
		if got != wantIface {
			t.Errorf("metric %q: interface=%q, want %q", s.Metric, got, wantIface)
		}
	}
}

// assertSampleByIface looks up a sample by "assetID:metric:interface" key.
func assertSampleByIface(
	t *testing.T,
	byKey map[string]telemetry.MetricSample,
	assetID, metric, wantUnit string,
	wantValue float64,
	iface string,
) {
	t.Helper()
	key := assetID + ":" + metric + ":" + iface
	s, ok := byKey[key]
	if !ok {
		t.Errorf("missing sample assetID=%q metric=%q interface=%q", assetID, metric, iface)
		return
	}
	if s.Unit != wantUnit {
		t.Errorf("sample %q/%q/%q: unit=%q, want %q", assetID, metric, iface, s.Unit, wantUnit)
	}
	if s.Value != wantValue {
		t.Errorf("sample %q/%q/%q: value=%v, want %v", assetID, metric, iface, s.Value, wantValue)
	}
	if s.CollectedAt.IsZero() {
		t.Errorf("sample %q/%q/%q: CollectedAt is zero", assetID, metric, iface)
	}
}

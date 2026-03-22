package bridge

import (
	"time"

	"github.com/labtether/labtether/internal/telemetry"
)

// NetworkInterfacesSource provides per-interface metrics from an asset.
type NetworkInterfacesSource interface {
	// AllNetworkInterfaces returns metrics for all known network interfaces.
	AllNetworkInterfaces() []NetworkInterfaceEntry
}

// NetworkInterfaceEntry holds per-interface metric values and identifying labels.
type NetworkInterfaceEntry struct {
	AssetID   string
	RXBytes   float64 // bytes/sec
	TXBytes   float64 // bytes/sec
	RXPackets float64
	TXPackets float64
	Labels    map[string]string // interface
}

// NetworkInterfacesBridge is a MetricsBridge that reads per-interface network
// metrics from a NetworkInterfacesSource and converts them to MetricSample objects.
type NetworkInterfacesBridge struct {
	source NetworkInterfacesSource
}

// NewNetworkInterfacesBridge creates a NetworkInterfacesBridge backed by the given source.
func NewNetworkInterfacesBridge(source NetworkInterfacesSource) *NetworkInterfacesBridge {
	return &NetworkInterfacesBridge{source: source}
}

// Name returns the bridge identifier.
func (b *NetworkInterfacesBridge) Name() string { return "network-interfaces" }

// Interval returns how often this bridge should be collected.
func (b *NetworkInterfacesBridge) Interval() time.Duration { return 30 * time.Second }

// Collect iterates all interfaces from the source and produces 4 MetricSamples
// per interface: rx/tx bytes per sec and rx/tx packets.
// Interfaces are differentiated by the interface label.
func (b *NetworkInterfacesBridge) Collect() []telemetry.MetricSample {
	entries := b.source.AllNetworkInterfaces()
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
				Metric:      telemetry.MetricInterfaceRXBytesPerSec,
				Unit:        "bytes_per_sec",
				Value:       e.RXBytes,
				CollectedAt: now,
				Labels:      labels,
			},
			telemetry.MetricSample{
				AssetID:     e.AssetID,
				Metric:      telemetry.MetricInterfaceTXBytesPerSec,
				Unit:        "bytes_per_sec",
				Value:       e.TXBytes,
				CollectedAt: now,
				Labels:      labels,
			},
			telemetry.MetricSample{
				AssetID:     e.AssetID,
				Metric:      telemetry.MetricInterfaceRXPackets,
				Unit:        "count",
				Value:       e.RXPackets,
				CollectedAt: now,
				Labels:      labels,
			},
			telemetry.MetricSample{
				AssetID:     e.AssetID,
				Metric:      telemetry.MetricInterfaceTXPackets,
				Unit:        "count",
				Value:       e.TXPackets,
				CollectedAt: now,
				Labels:      labels,
			},
		)
	}

	return out
}

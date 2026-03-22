package bridge

import (
	"time"

	"github.com/labtether/labtether/internal/telemetry"
)

// ServiceHealthSource provides web service health metrics from a monitor.
type ServiceHealthSource interface {
	// AllServiceHealth returns health metrics for all monitored services.
	AllServiceHealth() []ServiceHealthEntry
}

// ServiceHealthEntry holds per-service health metric values and identifying labels.
type ServiceHealthEntry struct {
	AssetID       string
	ResponseMs    float64
	UptimePercent float64
	Status        float64 // 0=down, 1=up
	Labels        map[string]string // service_name, service_url
}

// ServiceHealthBridge is a MetricsBridge that reads web service health metrics
// from a ServiceHealthSource and converts them to MetricSample objects.
type ServiceHealthBridge struct {
	source ServiceHealthSource
}

// NewServiceHealthBridge creates a ServiceHealthBridge backed by the given source.
func NewServiceHealthBridge(source ServiceHealthSource) *ServiceHealthBridge {
	return &ServiceHealthBridge{source: source}
}

// Name returns the bridge identifier.
func (b *ServiceHealthBridge) Name() string { return "service-health" }

// Interval returns how often this bridge should be collected.
func (b *ServiceHealthBridge) Interval() time.Duration { return 60 * time.Second }

// Collect iterates all service entries from the source and produces 3
// MetricSamples per service: response time, uptime percent, and status.
// Services are differentiated by service_name and service_url labels.
func (b *ServiceHealthBridge) Collect() []telemetry.MetricSample {
	entries := b.source.AllServiceHealth()
	if len(entries) == 0 {
		return nil
	}

	now := time.Now().UTC()
	out := make([]telemetry.MetricSample, 0, len(entries)*3)

	for _, e := range entries {
		labels := e.Labels

		out = append(out,
			telemetry.MetricSample{
				AssetID:     e.AssetID,
				Metric:      telemetry.MetricServiceResponseMs,
				Unit:        "milliseconds",
				Value:       e.ResponseMs,
				CollectedAt: now,
				Labels:      labels,
			},
			telemetry.MetricSample{
				AssetID:     e.AssetID,
				Metric:      telemetry.MetricServiceUptimePercent,
				Unit:        "percent",
				Value:       e.UptimePercent,
				CollectedAt: now,
				Labels:      labels,
			},
			telemetry.MetricSample{
				AssetID:     e.AssetID,
				Metric:      telemetry.MetricServiceStatus,
				Unit:        "status",
				Value:       e.Status,
				CollectedAt: now,
				Labels:      labels,
			},
		)
	}

	return out
}

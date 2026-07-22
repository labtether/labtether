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
	HasResponse   bool
	UptimePercent float64
	HasUptime     bool
	Status        float64           // 0=down, 1=up
	Labels        map[string]string // service_name, target (stable service ID; never the URL)
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
// Services are differentiated by bounded service_name and stable target-ID
// labels. The historical service_url descriptor label remains empty so
// credential-bearing URLs never enter telemetry.
func (b *ServiceHealthBridge) Collect() []telemetry.MetricSample {
	entries := b.source.AllServiceHealth()
	if len(entries) == 0 {
		return nil
	}

	now := time.Now().UTC()
	out := make([]telemetry.MetricSample, 0, len(entries)*3)

	for _, e := range entries {
		labels := e.Labels

		if e.HasResponse {
			out = append(out, telemetry.MetricSample{
				AssetID:     e.AssetID,
				Metric:      telemetry.MetricServiceResponseMs,
				Unit:        "milliseconds",
				Value:       e.ResponseMs,
				CollectedAt: now,
				Labels:      labels,
			})
		}
		if e.HasUptime {
			out = append(out, telemetry.MetricSample{
				AssetID:     e.AssetID,
				Metric:      telemetry.MetricServiceUptimePercent,
				Unit:        "percent",
				Value:       e.UptimePercent,
				CollectedAt: now,
				Labels:      labels,
			})
		}
		out = append(out, telemetry.MetricSample{
			AssetID:     e.AssetID,
			Metric:      telemetry.MetricServiceStatus,
			Unit:        "status",
			Value:       e.Status,
			CollectedAt: now,
			Labels:      labels,
		})
	}

	return out
}

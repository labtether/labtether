package bridge

import (
	"time"

	"github.com/labtether/labtether/internal/telemetry"
)

// MetricsBridge collects metrics from a single data source.
type MetricsBridge interface {
	Name() string
	Collect() []telemetry.MetricSample
	Interval() time.Duration
}

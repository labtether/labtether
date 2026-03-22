package model

import "time"

type MetricType string

const (
	MetricTypeGauge   MetricType = "gauge"
	MetricTypeCounter MetricType = "counter"
	MetricTypeRate    MetricType = "rate"
	MetricTypeState   MetricType = "state"
)

type MetricClass string

const (
	MetricClassUtilization  MetricClass = "utilization"
	MetricClassThroughput   MetricClass = "throughput"
	MetricClassTemperature  MetricClass = "temperature"
	MetricClassCapacity     MetricClass = "capacity"
	MetricClassAvailability MetricClass = "availability"
)

type MetricDescriptor struct {
	ID          string      `json:"id"`
	Unit        string      `json:"unit"`
	Type        MetricType  `json:"type"`
	Class       MetricClass `json:"class"`
	TargetKinds []string    `json:"target_kinds,omitempty"`
}

type MetricSample struct {
	ResourceID         string            `json:"resource_id"`
	MetricID           string            `json:"metric_id"`
	Unit               string            `json:"unit"`
	Value              float64           `json:"value"`
	Labels             map[string]string `json:"labels,omitempty"`
	CollectedAt        time.Time         `json:"collected_at"`
	ProviderInstanceID string            `json:"provider_instance_id,omitempty"`
}

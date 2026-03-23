package collectors

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Standard metric keys shared between agent endpoint-helper and hub collector paths.
const (
	MetricCPUPercent       = "cpu_used_percent"
	MetricMemoryPercent    = "memory_used_percent"
	MetricDiskPercent      = "disk_used_percent"
	MetricTempCelsius      = "temperature_celsius"
	MetricNetRXBytesPerSec = "network_rx_bytes_per_sec"
	MetricNetTXBytesPerSec = "network_tx_bytes_per_sec"
)

// CollectorSample represents a single metric sample parsed from collector output.
type CollectorSample struct {
	Metric    string    `json:"metric"`
	Value     float64   `json:"value"`
	AssetID   string    `json:"asset_id,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// ParseCollectorOutput parses collector output into telemetry samples.
// Supports "json" and "keyvalue" formats.
func ParseCollectorOutput(output string, format string, assetID string) ([]CollectorSample, error) {
	now := time.Now().UTC()

	switch strings.ToLower(format) {
	case "json":
		return parseJSONOutput(output, assetID, now)
	case "keyvalue", "kv":
		return parseKeyValueOutput(output, assetID, now)
	default:
		// Auto-detect: try JSON first, then key-value
		samples, err := parseJSONOutput(output, assetID, now)
		if err == nil && len(samples) > 0 {
			return samples, nil
		}
		return parseKeyValueOutput(output, assetID, now)
	}
}

// parseJSONOutput tries to parse JSON output as either a flat object of metric:value pairs
// or an array of {metric, value} objects.
func parseJSONOutput(output string, assetID string, now time.Time) ([]CollectorSample, error) {
	output = strings.TrimSpace(output)

	// Try flat object: {"cpu_used_percent": 45.2, "memory_used_percent": 60.1}
	var flat map[string]any
	if err := json.Unmarshal([]byte(output), &flat); err == nil {
		var samples []CollectorSample
		for k, v := range flat {
			val, ok := toFloat(v)
			if !ok {
				continue
			}
			samples = append(samples, CollectorSample{
				Metric:    NormalizeMetricKey(k),
				Value:     val,
				AssetID:   assetID,
				Timestamp: now,
			})
		}
		if len(samples) > 0 {
			return samples, nil
		}
	}

	// Try array: [{"metric": "cpu", "value": 45.2}, ...]
	var arr []map[string]any
	if err := json.Unmarshal([]byte(output), &arr); err == nil {
		var samples []CollectorSample
		for _, entry := range arr {
			metric, _ := entry["metric"].(string)
			val, ok := toFloat(entry["value"])
			if metric == "" || !ok {
				continue
			}
			samples = append(samples, CollectorSample{
				Metric:    NormalizeMetricKey(metric),
				Value:     val,
				AssetID:   assetID,
				Timestamp: now,
			})
		}
		return samples, nil
	}

	return nil, nil
}

var kvPattern = regexp.MustCompile(`^([a-zA-Z_][a-zA-Z0-9_./-]*)[\s:=]+([0-9]+\.?[0-9]*)`)

// parseKeyValueOutput parses lines of "key=value" or "key: value" format.
func parseKeyValueOutput(output string, assetID string, now time.Time) ([]CollectorSample, error) {
	var samples []CollectorSample
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		matches := kvPattern.FindStringSubmatch(line)
		if len(matches) < 3 {
			continue
		}
		val, err := strconv.ParseFloat(matches[2], 64)
		if err != nil {
			continue
		}
		samples = append(samples, CollectorSample{
			Metric:    NormalizeMetricKey(matches[1]),
			Value:     val,
			AssetID:   assetID,
			Timestamp: now,
		})
	}
	return samples, nil
}

// NormalizeMetricKey maps common metric name variants to canonical keys.
func NormalizeMetricKey(key string) string {
	k := strings.ToLower(strings.TrimSpace(key))
	switch k {
	case "cpu", "cpu_percent", "cpu_usage", "cpu_used_percent":
		return MetricCPUPercent
	case "memory", "mem", "mem_percent", "memory_percent", "memory_usage", "memory_used_percent":
		return MetricMemoryPercent
	case "disk", "disk_percent", "disk_usage", "disk_used_percent":
		return MetricDiskPercent
	case "temp", "temperature", "temp_celsius", "temperature_celsius":
		return MetricTempCelsius
	case "net_rx", "network_rx", "network_rx_bytes_per_sec":
		return MetricNetRXBytesPerSec
	case "net_tx", "network_tx", "network_tx_bytes_per_sec":
		return MetricNetTXBytesPerSec
	default:
		return k
	}
}

func toFloat(v any) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	case string:
		f, err := strconv.ParseFloat(val, 64)
		return f, err == nil
	default:
		return 0, false
	}
}

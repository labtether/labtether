package proxmox

import (
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	telemetrypkg "github.com/labtether/labtether/internal/telemetry"
)

func ParsePositiveInt(raw string) (int, bool) {
	parsed, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || parsed <= 0 {
		return 0, false
	}
	return parsed, true
}

func ParseMetadataFloat(metadata map[string]string, keys ...string) (float64, bool) {
	for _, key := range keys {
		raw := strings.TrimSpace(metadata[key])
		if raw == "" {
			continue
		}
		parsed, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			continue
		}
		return parsed, true
	}
	return 0, false
}

func ParseAnyInt64(value any) (int64, bool) {
	switch typed := value.(type) {
	case int64:
		return typed, true
	case int32:
		return int64(typed), true
	case int:
		return int64(typed), true
	case float64:
		if !math.IsNaN(typed) && !math.IsInf(typed, 0) {
			return int64(typed), true
		}
	case float32:
		if !math.IsNaN(float64(typed)) && !math.IsInf(float64(typed), 0) {
			return int64(typed), true
		}
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		if err == nil && !math.IsNaN(parsed) && !math.IsInf(parsed, 0) {
			return int64(parsed), true
		}
	}
	return 0, false
}

func AnalyzeDiskGrowth(points []telemetrypkg.Point) (float64, string, int64) {
	if len(points) == 0 {
		return 0, "low", 0
	}

	latestTS := points[len(points)-1].TS
	if len(points) < 2 {
		return 0, "low", latestTS
	}

	rates := make([]float64, 0, len(points)-1)
	for i := 1; i < len(points); i++ {
		deltaTS := points[i].TS - points[i-1].TS
		if deltaTS <= 0 {
			continue
		}
		deltaDays := float64(deltaTS) / float64(24*time.Hour/time.Second)
		rate := (points[i].Value - points[i-1].Value) / deltaDays
		if math.IsNaN(rate) || math.IsInf(rate, 0) {
			continue
		}
		rates = append(rates, rate)
	}
	if len(rates) == 0 {
		return 0, "low", latestTS
	}

	median := MedianFloat64(rates)
	stdDev := StdDevFloat64(rates)
	absMedian := math.Abs(median)
	confidence := "low"
	if len(rates) >= 6 && absMedian >= 0.2 && stdDev <= math.Max(absMedian*0.35, 0.25) {
		confidence = "high"
	} else if len(rates) >= 4 {
		confidence = "medium"
	}
	return median, confidence, latestTS
}

func ComputeStorageRisk(pool ProxmoxStorageInsightPool) (int, string, []string) {
	score := 0
	reasons := make([]string, 0, 4)

	if !ProxmoxStorageHealthOK(pool.Health) {
		score += 60
		reasons = append(reasons, "Pool health is degraded.")
	}
	if pool.UsedPercent != nil {
		switch {
		case *pool.UsedPercent >= 90:
			score += 30
			reasons = append(reasons, "Used capacity is above 90%.")
		case *pool.UsedPercent >= 80:
			score += 18
			reasons = append(reasons, "Used capacity is above 80%.")
		}
	}
	if pool.Forecast.DaysToFull != nil {
		switch {
		case *pool.Forecast.DaysToFull <= 7:
			score += 35
			reasons = append(reasons, "Forecasted full in under 7 days.")
		case *pool.Forecast.DaysToFull <= 30:
			score += 20
			reasons = append(reasons, "Forecasted full in under 30 days.")
		case *pool.Forecast.DaysToFull <= 90:
			score += 8
			reasons = append(reasons, "Forecasted full in under 90 days.")
		}
	}
	if pool.TelemetryStale {
		score += 15
		reasons = append(reasons, "Telemetry data is stale.")
	}

	if score > 100 {
		score = 100
	}
	if len(reasons) == 0 {
		reasons = append(reasons, "No immediate storage risk detected.")
	}

	state := "healthy"
	switch {
	case score >= 75:
		state = "critical"
	case score >= 50:
		state = "action"
	case score >= 25:
		state = "watch"
	}
	return score, state, reasons
}

func ProxmoxStorageHealthOK(health string) bool {
	switch strings.ToUpper(strings.TrimSpace(health)) {
	case "ONLINE", "OK", "ACTIVE":
		return true
	default:
		return false
	}
}

func MedianFloat64(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	cloned := make([]float64, len(values))
	copy(cloned, values)
	sort.Float64s(cloned)
	mid := len(cloned) / 2
	if len(cloned)%2 == 0 {
		return (cloned[mid-1] + cloned[mid]) / 2
	}
	return cloned[mid]
}

func StdDevFloat64(values []float64) float64 {
	if len(values) < 2 {
		return 0
	}
	sum := 0.0
	for _, value := range values {
		sum += value
	}
	mean := sum / float64(len(values))
	varianceSum := 0.0
	for _, value := range values {
		delta := value - mean
		varianceSum += delta * delta
	}
	variance := varianceSum / float64(len(values))
	return math.Sqrt(variance)
}

func Int64Ptr(value int64) *int64 {
	return &value
}

func IntPtr(value int) *int {
	return &value
}

func Float64Ptr(value float64) *float64 {
	return &value
}

func ClampPercent(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}

func RoundToSingleDecimal(value float64) float64 {
	return math.Round(value*10) / 10
}

func ParseStorageInsightsWindow(raw string) time.Duration {
	const (
		minWindow = 24 * time.Hour
		maxWindow = 30 * 24 * time.Hour
		fallback  = 7 * 24 * time.Hour
	)

	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return fallback
	}

	parsed := time.Duration(0)
	if strings.HasSuffix(raw, "d") {
		days, err := strconv.Atoi(strings.TrimSpace(strings.TrimSuffix(raw, "d")))
		if err != nil || days <= 0 {
			return fallback
		}
		parsed = time.Duration(days) * 24 * time.Hour
	} else {
		value, err := time.ParseDuration(raw)
		if err != nil {
			return fallback
		}
		parsed = value
	}

	if parsed < minWindow || parsed > maxWindow {
		return fallback
	}
	return parsed
}

func FormatStorageInsightsWindow(window time.Duration) string {
	if window%(24*time.Hour) == 0 {
		return strconv.Itoa(int(window/(24*time.Hour))) + "d"
	}
	return window.String()
}

func StorageInsightsStep(window time.Duration) time.Duration {
	switch {
	case window >= 7*24*time.Hour:
		return time.Hour
	case window >= 24*time.Hour:
		return 30 * time.Minute
	default:
		return 5 * time.Minute
	}
}

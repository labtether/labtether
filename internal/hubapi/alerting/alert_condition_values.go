package alerting

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/telemetry"
)

func toFloat64(v any) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return finiteFloat64(val)
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
		if err != nil {
			return 0, false
		}
		return finiteFloat64(f)
	default:
		return 0, false
	}
}

func finiteFloat64(value float64) (float64, bool) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false
	}
	return value, true
}

func conditionString(condition map[string]any, key string) string {
	if condition == nil {
		return ""
	}
	if value, ok := condition[key]; ok {
		if asString, ok := value.(string); ok {
			return asString
		}
	}
	return ""
}

func conditionStringMap(raw any) map[string]string {
	switch typed := raw.(type) {
	case map[string]string:
		out := make(map[string]string, len(typed))
		for key, value := range typed {
			normalizedKey := strings.TrimSpace(key)
			normalizedValue := strings.TrimSpace(value)
			if normalizedKey == "" || normalizedValue == "" {
				continue
			}
			out[normalizedKey] = normalizedValue
		}
		return out
	case map[string]any:
		out := make(map[string]string, len(typed))
		for key, value := range typed {
			normalizedKey := strings.TrimSpace(key)
			if normalizedKey == "" {
				continue
			}
			switch converted := value.(type) {
			case string:
				normalizedValue := strings.TrimSpace(converted)
				if normalizedValue == "" {
					continue
				}
				out[normalizedKey] = normalizedValue
			case fmt.Stringer:
				normalizedValue := strings.TrimSpace(converted.String())
				if normalizedValue == "" {
					continue
				}
				out[normalizedKey] = normalizedValue
			default:
				continue
			}
		}
		return out
	default:
		return map[string]string{}
	}
}

func uniqueAlertAssetIDs(targetAssets []assets.Asset) []string {
	out := make([]string, 0, len(targetAssets))
	seen := make(map[string]struct{}, len(targetAssets))
	for _, targetAsset := range targetAssets {
		assetID := strings.TrimSpace(targetAsset.ID)
		if assetID == "" {
			continue
		}
		if _, ok := seen[assetID]; ok {
			continue
		}
		seen[assetID] = struct{}{}
		out = append(out, assetID)
	}
	return out
}

func aggregatePoints(points []telemetry.Point, agg string) float64 {
	if len(points) == 0 {
		return 0
	}
	switch agg {
	case "max":
		max := points[0].Value
		for _, p := range points[1:] {
			if p.Value > max {
				max = p.Value
			}
		}
		return max
	case "min":
		min := points[0].Value
		for _, p := range points[1:] {
			if p.Value < min {
				min = p.Value
			}
		}
		return min
	case "sum":
		sum := 0.0
		for _, p := range points {
			sum += p.Value
		}
		return sum
	default: // avg
		sum := 0.0
		for _, p := range points {
			sum += p.Value
		}
		return sum / float64(len(points))
	}
}

func compareThreshold(value float64, operator string, threshold float64) bool {
	switch operator {
	case ">":
		return value > threshold
	case ">=":
		return value >= threshold
	case "<":
		return value < threshold
	case "<=":
		return value <= threshold
	case "==":
		return value == threshold
	default:
		return value > threshold
	}
}

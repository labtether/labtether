package alerting

import "errors"

// metricThresholdValue reads the console/template value key and the historical
// API threshold key. Invalid persisted conditions must not become a zero limit.
func metricThresholdValue(condition map[string]any) (float64, error) {
	value, hasValue := condition["value"]
	legacy, hasLegacy := condition["threshold"]
	if !hasValue && !hasLegacy {
		return 0, errors.New("metric condition requires value or threshold")
	}
	var threshold float64
	if hasValue {
		parsed, ok := toFloat64(value)
		if !ok {
			return 0, errors.New("metric condition value must be a finite number")
		}
		threshold = parsed
	}
	if hasLegacy {
		parsed, ok := toFloat64(legacy)
		if !ok {
			return 0, errors.New("metric condition threshold must be a finite number")
		}
		if hasValue && threshold != parsed {
			return 0, errors.New("metric condition value and threshold must agree")
		}
		threshold = parsed
	}
	return threshold, nil
}

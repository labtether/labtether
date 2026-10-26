package alerting

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/alerts"
)

// Explicit silence limits describe age, not a metric aggregation window. Old
// rules without these keys retain their historical WindowSeconds behavior.
func alertStalenessCondition(rule alerts.Rule) (time.Duration, string, error) {
	field := ""
	switch rule.Kind {
	case alerts.RuleKindHeartbeatStale:
		field = "max_stale_seconds"
	case alerts.RuleKindMetricDeadman:
		field = "max_silence_seconds"
	default:
		return 0, "", nil
	}
	seconds := rule.WindowSeconds
	if raw, present := rule.Condition[field]; present {
		value, ok := toFloat64(raw)
		if !ok || value <= 0 || math.Trunc(value) != value || value > alerts.MaxDurationSeconds {
			return 0, "", fmt.Errorf("staleness condition %s must be a positive whole number of seconds no greater than %d", field, alerts.MaxDurationSeconds)
		}
		seconds = int(value)
	}
	metric := ""
	if rule.Kind == alerts.RuleKindMetricDeadman {
		if raw, present := rule.Condition["metric"]; present {
			var ok bool
			metric, ok = raw.(string)
			metric = strings.TrimSpace(metric)
			if !ok || metric == "" {
				return 0, "", fmt.Errorf("staleness condition metric must be a nonempty string when provided")
			}
		}
	}
	return alerts.DurationFromSeconds(seconds, time.Duration(alerts.DefaultWindowSeconds)*time.Second), metric, nil
}

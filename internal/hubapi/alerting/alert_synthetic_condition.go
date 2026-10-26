package alerting

import (
	"errors"
	"math"
)

// The PostgreSQL synthetic history reader returns at most 500 results. A larger
// streak could never be established by that store; do not silently truncate it.
const maxSyntheticConsecutiveFailures = 500

func syntheticFailureCount(condition map[string]any) (int, error) {
	raw, present := condition["consecutive_failures"]
	if !present {
		return 3, nil
	}
	value, ok := toFloat64(raw)
	if !ok || value < 1 || value > maxSyntheticConsecutiveFailures || math.Trunc(value) != value {
		return 0, errors.New("synthetic condition consecutive_failures must be a finite whole number between 1 and 500")
	}
	return int(value), nil
}

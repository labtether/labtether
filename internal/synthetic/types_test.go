package synthetic

import (
	"errors"
	"testing"
)

func TestCreateIntervalSecondsDefaultsOnlyWhenOmitted(t *testing.T) {
	got, err := CreateIntervalSeconds(0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != DefaultIntervalSeconds {
		t.Fatalf("interval = %d, want %d", got, DefaultIntervalSeconds)
	}
}

func TestValidateIntervalSecondsRejectsValuesOutsideStorageBounds(t *testing.T) {
	for _, value := range []int{0, -1, MaxIntervalSeconds + 1} {
		if err := ValidateIntervalSeconds(value); !errors.Is(err, ErrInvalidInterval) {
			t.Fatalf("ValidateIntervalSeconds(%d) error = %v, want ErrInvalidInterval", value, err)
		}
	}
}

func TestIntervalDurationRejectsOutOfRangeValuesBeforeConversion(t *testing.T) {
	if got := IntervalDuration(MaxIntervalSeconds + 1); got != 0 {
		t.Fatalf("duration = %s, want 0", got)
	}
	if got := IntervalDuration(MaxIntervalSeconds); got <= 0 {
		t.Fatalf("duration = %s, want positive", got)
	}
}

package shared

import (
	"math"
	"testing"
	"time"
)

func TestCollectorConfigDurationHandlesFractionalSeconds(t *testing.T) {
	got := CollectorConfigDuration(map[string]any{"timeout": 0.5}, "timeout", 15*time.Second)
	if got != 500*time.Millisecond {
		t.Fatalf("CollectorConfigDuration() = %s, want 500ms", got)
	}
}

func TestCollectorConfigDurationRejectsUnsafeSeconds(t *testing.T) {
	fallback := 15 * time.Second
	for _, raw := range []any{math.NaN(), math.Inf(1), 1e100, "999999999999999999999999999999"} {
		got := CollectorConfigDuration(map[string]any{"timeout": raw}, "timeout", fallback)
		if got != fallback {
			t.Fatalf("CollectorConfigDuration(%#v) = %s, want %s", raw, got, fallback)
		}
	}
}

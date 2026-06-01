package alerting

import (
	"testing"
	"time"
)

func TestSyntheticTimeoutHandlesFractionalSeconds(t *testing.T) {
	got := syntheticTimeout(map[string]any{"timeout_seconds": 0.25}, 10*time.Second)
	if got != 250*time.Millisecond {
		t.Fatalf("syntheticTimeout() = %s, want 250ms", got)
	}
}

func TestSyntheticTimeoutRejectsUnsafeSeconds(t *testing.T) {
	fallback := 10 * time.Second
	got := syntheticTimeout(map[string]any{"timeout_seconds": 1e100}, fallback)
	if got != fallback {
		t.Fatalf("syntheticTimeout(overflow) = %s, want %s", got, fallback)
	}
}

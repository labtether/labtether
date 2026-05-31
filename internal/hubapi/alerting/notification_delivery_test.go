package alerting

import (
	"math"
	"testing"
)

func TestNotificationAnyToStringAvoidsUnsafeFloatIntegerConversion(t *testing.T) {
	if got := notificationAnyToString(1e100); got != "1e+100" {
		t.Fatalf("notificationAnyToString(1e100) = %q, want 1e+100", got)
	}
	if got := notificationAnyToString(math.Inf(1)); got != "" {
		t.Fatalf("notificationAnyToString(+Inf) = %q, want empty", got)
	}
	if got := notificationAnyToString(float64(42)); got != "42" {
		t.Fatalf("notificationAnyToString(42) = %q, want 42", got)
	}
}

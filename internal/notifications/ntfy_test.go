package notifications

import "testing"

func TestNotificationPriorityRejectsUnsafeFloatConversions(t *testing.T) {
	for _, raw := range []any{1e100, 1.5} {
		if got, ok := notificationPriority(raw); ok {
			t.Fatalf("notificationPriority(%#v) = %d, true; want false", raw, got)
		}
	}

	if got, ok := notificationPriority(float64(4)); !ok || got != 4 {
		t.Fatalf("notificationPriority(4) = %d, %v; want 4, true", got, ok)
	}
}

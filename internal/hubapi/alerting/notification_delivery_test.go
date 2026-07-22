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

func TestBuildEmailNotificationPayloadUsesTitleWhenTestHasNoRuleName(t *testing.T) {
	payload, err := buildEmailNotificationPayload(map[string]any{
		"to":             "qa@example.invalid",
		"subject_prefix": "[LabTether]",
	}, map[string]any{
		"title":    "LabTether test notification",
		"severity": "low",
		"state":    "test",
		"text":     "This is a test notification.",
	})
	if err != nil {
		t.Fatalf("build email test payload: %v", err)
	}
	if got, want := payload["subject"], "[LabTether] [LOW] LabTether test notification (test)"; got != want {
		t.Fatalf("subject = %q, want %q", got, want)
	}
}

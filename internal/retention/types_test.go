package retention

import (
	"testing"
	"time"
)

func TestParseDurationRejectsOverflowingDayAndWeekCounts(t *testing.T) {
	for _, raw := range []string{
		"2000000000w",
		"200000000000d",
		"999999999999999999999999w",
	} {
		t.Run(raw, func(t *testing.T) {
			if got, err := ParseDuration(raw); err == nil {
				t.Fatalf("ParseDuration(%q) = %s, nil error; want overflow rejection", raw, got)
			}
		})
	}
}

func TestParseDurationKeepsValidDayWeekAndNativeDurations(t *testing.T) {
	cases := []struct {
		raw  string
		want time.Duration
	}{
		{raw: "2w", want: 14 * 24 * time.Hour},
		{raw: "14d", want: 14 * 24 * time.Hour},
		{raw: "90m", want: 90 * time.Minute},
	}

	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			got, err := ParseDuration(tc.raw)
			if err != nil {
				t.Fatalf("ParseDuration(%q) error = %v", tc.raw, err)
			}
			if got != tc.want {
				t.Fatalf("ParseDuration(%q) = %s, want %s", tc.raw, got, tc.want)
			}
		})
	}
}

// The audit trail must not be prunable to hours; that would let an operator
// erase the record of their own actions.
func TestAuditWindowHasEnforcedFloor(t *testing.T) {
	short := DefaultSettings()
	short.AuditWindow = time.Hour
	if err := Validate(short); err == nil {
		t.Fatal("Validate accepted a one-hour audit window")
	}
	atFloor := DefaultSettings()
	atFloor.AuditWindow = MinAuditWindow
	if err := Validate(atFloor); err != nil {
		t.Fatalf("Validate rejected the minimum audit window: %v", err)
	}
	// Values stored before the floor existed are raised, so pruning honours it.
	if got := Normalize(short).AuditWindow; got != MinAuditWindow {
		t.Fatalf("Normalize audit window = %s, want %s", got, MinAuditWindow)
	}
	for _, preset := range Presets() {
		if err := Validate(preset.Settings); err != nil {
			t.Fatalf("preset %s violates the audit floor: %v", preset.ID, err)
		}
	}
}

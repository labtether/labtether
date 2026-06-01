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

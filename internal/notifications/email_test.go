package notifications

import (
	"math"
	"testing"
)

func TestSMTPPortFromConfigDefaultsAndAcceptsValidPorts(t *testing.T) {
	cases := []struct {
		name string
		raw  any
		want int
	}{
		{name: "missing", raw: nil, want: defaultSMTPPort},
		{name: "int", raw: 2525, want: 2525},
		{name: "int64", raw: int64(465), want: 465},
		{name: "float64 integer", raw: float64(587), want: 587},
		{name: "string", raw: " 1025 ", want: 1025},
		{name: "blank string", raw: " ", want: defaultSMTPPort},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := smtpPortFromConfig(tc.raw)
			if err != nil {
				t.Fatalf("smtpPortFromConfig(%v) error = %v", tc.raw, err)
			}
			if got != tc.want {
				t.Fatalf("smtpPortFromConfig(%v) = %d, want %d", tc.raw, got, tc.want)
			}
		})
	}
}

func TestSMTPPortFromConfigRejectsUnsafeValues(t *testing.T) {
	cases := []struct {
		name string
		raw  any
	}{
		{name: "zero", raw: 0},
		{name: "negative", raw: -1},
		{name: "too large int", raw: 65536},
		{name: "fractional", raw: 25.5},
		{name: "huge float", raw: 1e100},
		{name: "infinite", raw: math.Inf(1)},
		{name: "nan", raw: math.NaN()},
		{name: "malformed string", raw: "587abc"},
		{name: "unsupported", raw: []string{"587"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got, err := smtpPortFromConfig(tc.raw); err == nil {
				t.Fatalf("smtpPortFromConfig(%v) = %d, nil error; want rejection", tc.raw, got)
			}
		})
	}
}

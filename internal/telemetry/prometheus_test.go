package telemetry

import "testing"

func TestParsePromSampleValueRejectsNonFinite(t *testing.T) {
	for _, raw := range []any{"NaN", "Inf", "-Inf"} {
		if got, err := parsePromSampleValue(raw); err == nil {
			t.Fatalf("parsePromSampleValue(%#v) = %v, nil; want error", raw, got)
		}
	}
}

func TestParsePromSampleTSRejectsNonFinite(t *testing.T) {
	for _, raw := range []any{"NaN", "Inf", "-Inf"} {
		if got, err := parsePromSampleTS(raw); err == nil {
			t.Fatalf("parsePromSampleTS(%#v) = %v, nil; want error", raw, got)
		}
	}
}

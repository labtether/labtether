package alerting

import "testing"

func TestToFloat64RejectsNonFiniteValues(t *testing.T) {
	for _, raw := range []any{float64(1.5), "2.5"} {
		if _, ok := toFloat64(raw); !ok {
			t.Fatalf("toFloat64(%#v) rejected finite value", raw)
		}
	}

	for _, raw := range []any{"NaN", "Inf", "-Inf"} {
		if got, ok := toFloat64(raw); ok {
			t.Fatalf("toFloat64(%#v) = %v, true; want false", raw, got)
		}
	}
}

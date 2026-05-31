package resources

import "testing"

func TestParseProcessFloatRejectsNonFiniteValues(t *testing.T) {
	if got := parseProcessFloat("12.5"); got != 12.5 {
		t.Fatalf("parseProcessFloat(12.5) = %v, want 12.5", got)
	}

	for _, raw := range []string{"NaN", "Inf", "-Inf"} {
		if got := parseProcessFloat(raw); got != 0 {
			t.Fatalf("parseProcessFloat(%q) = %v, want 0", raw, got)
		}
	}
}

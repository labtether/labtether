package proxmox

import "testing"

func TestParseMetadataFloatRejectsNonFiniteValues(t *testing.T) {
	metadata := map[string]string{
		"first":  "NaN",
		"second": "Inf",
		"third":  "42.5",
	}

	got, ok := ParseMetadataFloat(metadata, "first", "second", "third")
	if !ok || got != 42.5 {
		t.Fatalf("ParseMetadataFloat() = %v, %v; want 42.5, true", got, ok)
	}
}

func TestParseAnyInt64RejectsUnsafeFloatConversions(t *testing.T) {
	for _, raw := range []any{1e100, "1e100"} {
		if got, ok := ParseAnyInt64(raw); ok {
			t.Fatalf("ParseAnyInt64(%#v) = %d, true; want false", raw, got)
		}
	}

	if got, ok := ParseAnyInt64(42.5); !ok || got != 42 {
		t.Fatalf("ParseAnyInt64(42.5) = %d, %v; want 42, true", got, ok)
	}
	if got, ok := ParseAnyInt64("1e3"); !ok || got != 1000 {
		t.Fatalf("ParseAnyInt64(1e3) = %d, %v; want 1000, true", got, ok)
	}
	if got, ok := ParseAnyInt64(float64(42)); !ok || got != 42 {
		t.Fatalf("ParseAnyInt64(42) = %d, %v; want 42, true", got, ok)
	}
}

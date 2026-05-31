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

package assets

import "testing"

// A device renamed to "nas01\r\nBcc: x@example.com" was stored as given and
// showed on two lines. Typed names with control characters are refused;
// names from agents and connectors are stored as one line.
func TestDeviceNamesAreOneLine(t *testing.T) {
	for _, bad := range []string{"nas01\r\nBcc: x@example.com", "nas01\tbox", "nas\x0001", "nas01\u0085"} {
		if err := ValidateName(bad); err != ErrNameHasControlCharacters {
			t.Errorf("ValidateName(%q) = %v, want the refusal", bad, err)
		}
	}
	if err := ValidateName("nas01 · rack 2"); err != nil {
		t.Errorf("ValidateName(plain) = %v", err)
	}
	if got := OneLineName("  nas01\r\n\tBcc:  x@example.com "); got != "nas01 Bcc: x@example.com" {
		t.Errorf("OneLineName = %q", got)
	}
}

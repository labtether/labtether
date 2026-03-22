package securityruntime

import (
	"strings"
	"testing"
)

func TestSanitizeLogValueEscapesControlCharacters(t *testing.T) {
	got := SanitizeLogValue("line1\nline2\r\n\tok")
	if strings.Contains(got, "\n") || strings.Contains(got, "\r") || strings.Contains(got, "\t") {
		t.Fatalf("expected escaped control chars, got %q", got)
	}
	if !strings.Contains(got, "\\n") {
		t.Fatalf("expected escaped newline marker, got %q", got)
	}
}

func TestSanitizeLogValueTruncates(t *testing.T) {
	t.Setenv(envLogSanitizeMaxLength, "64")
	got := SanitizeLogValue(strings.Repeat("x", 160))
	if !strings.Contains(got, "...(truncated)") {
		t.Fatalf("expected truncation marker, got %q", got)
	}
}

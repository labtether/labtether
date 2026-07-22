package notifications

import (
	"strings"
	"testing"
)

func TestSanitizeDeliveryErrorMessageRemovesCredentialMaterialAndControls(t *testing.T) {
	message := "request https://user:pass@example.com/private/path?token=synthetic-value failed authorization=Bearer synthetic-bearer api_key=synthetic-key AUTH PLAIN synthetic-auth-blob v2:syntheticcipher\r\ninjected"
	safe := SanitizeDeliveryErrorMessage(message)

	for _, forbidden := range []string{
		"user:pass",
		"/private/path",
		"synthetic-value",
		"synthetic-bearer",
		"synthetic-key",
		"synthetic-auth-blob",
		"syntheticcipher",
		"\r",
		"\n",
	} {
		if strings.Contains(safe, forbidden) {
			t.Fatal("sanitized delivery error retained credential or control material")
		}
	}
	if !strings.Contains(safe, "https://example.com/[redacted]") {
		t.Fatalf("sanitized delivery error = %q, want host-only URL", safe)
	}
}

func TestSanitizeDeliveryErrorMessageIsBounded(t *testing.T) {
	safe := SanitizeDeliveryErrorMessage(strings.Repeat("x", maxSanitizedDeliveryErrorLength+100))
	if len(safe) != maxSanitizedDeliveryErrorLength {
		t.Fatalf("sanitized error length = %d, want %d", len(safe), maxSanitizedDeliveryErrorLength)
	}
}

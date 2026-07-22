package terminal

import (
	"testing"

	"github.com/labtether/labtether/internal/hubapi/shared"
)

func TestQuickSessionDefaultsToStrictHostKeyVerification(t *testing.T) {
	t.Setenv(shared.EnvAllowInsecureSSHHostKeys, "")
	if !effectiveQuickSessionStrictHostKey(false) {
		t.Fatal("zero-value quick-connect setting must not disable SSH host-key verification")
	}
}

func TestQuickSessionInsecureModeRequiresGlobalAcknowledgement(t *testing.T) {
	t.Setenv(shared.EnvAllowInsecureSSHHostKeys, "true")
	if effectiveQuickSessionStrictHostKey(false) {
		t.Fatal("expected explicitly acknowledged quick-connect insecure mode")
	}
	if !effectiveQuickSessionStrictHostKey(true) {
		t.Fatal("an explicit strict setting must remain strict")
	}
}

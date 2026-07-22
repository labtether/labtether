package shared

import "testing"

func TestInsecureSSHHostKeysRequireExplicitAcknowledgement(t *testing.T) {
	t.Setenv(EnvAllowInsecureSSHHostKeys, "")
	if InsecureSSHHostKeysAllowed() {
		t.Fatal("insecure SSH host keys must be disabled by default")
	}

	t.Setenv(EnvAllowInsecureSSHHostKeys, "true")
	if !InsecureSSHHostKeysAllowed() {
		t.Fatal("expected explicit insecure SSH host-key acknowledgement to be honored")
	}
}

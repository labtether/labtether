package terminal

import (
	"strings"

	"golang.org/x/crypto/ssh"

	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/terminal"
)

// pinnedHostKeyAlgorithms limits the host-key algorithms the client offers to
// the type of a pinned host key. A server with several host keys presents
// the one the client prefers; Go prefers ECDSA over ed25519, so a pinned
// ed25519 key would otherwise fail against a server that also has ECDSA.
// It returns nil (Go's defaults) when no full key is pinned or checking is
// off.
func pinnedHostKeyAlgorithms(cfg *terminal.SSHConfig) []string {
	if cfg == nil || (!cfg.StrictHostKey && shared.InsecureSSHHostKeysAllowed()) {
		return nil
	}
	key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(strings.TrimSpace(cfg.HostKey)))
	if err != nil {
		return nil
	}
	if key.Type() == ssh.KeyAlgoRSA {
		return []string{ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSASHA256}
	}
	return []string{key.Type()}
}

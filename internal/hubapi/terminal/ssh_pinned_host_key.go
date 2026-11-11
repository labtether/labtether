package terminal

import (
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/terminal"
)

// pinnedHostKeyAlgorithms limits the host-key algorithms the client offers to
// the type of a pinned host key (see shared.PinnedHostKeyAlgorithms). It
// returns nil (Go's defaults) when no full key is pinned or checking is off.
func pinnedHostKeyAlgorithms(cfg *terminal.SSHConfig) []string {
	if cfg == nil {
		return nil
	}
	return shared.PinnedHostKeyAlgorithms(cfg.StrictHostKey, cfg.HostKey)
}

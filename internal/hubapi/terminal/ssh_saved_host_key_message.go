package terminal

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/terminal"
)

// firstAgentVersionWithSSHHostKeys is the first Go agent release that reports
// the device's SSH host keys when it installs the Hub SSH key.
var firstAgentVersionWithSSHHostKeys = [3]int{1, 7, 1}

type savedSSHHostKeyMissingError struct{ text string }

func (e savedSSHHostKeyMissingError) Error() string { return e.text }
func (e savedSSHHostKeyMissingError) Unwrap() error { return errSavedSSHHostKeyMissing }

// sshHostKeyMissingError states what the Hub knows about why it has no host
// key for an SSH login: an agent version that cannot report it, or otherwise
// that the key is unknown and where to set it. fromConnectEntry is true for an
// SSH entry saved in the Connect panel, false for the saved Hub key login.
func (d *Deps) sshHostKeyMissingError(assetID, host string, fromConnectEntry bool) error {
	if version := d.assetAgentVersion(assetID); version != "" && agentVersionBefore(version, firstAgentVersionWithSSHHostKeys) {
		return savedSSHHostKeyMissingError{text: fmt.Sprintf(
			"The agent on this device is version %s, which does not report the device's SSH host key, so the Hub cannot verify the device. "+
				"Update the agent to 1.7.1 or later; the Hub takes the key when the agent reconnects.", version)}
	}
	setIt := "To set it yourself, add SSH in the device's Connect panel with the address and host key."
	if fromConnectEntry {
		setIt = "To set it yourself, enter it in the Host key field of the device's SSH entry in its Connect panel."
	}
	return savedSSHHostKeyMissingError{text: fmt.Sprintf(
		"The Hub does not know this device's SSH host key for %s, so it cannot verify the device. "+
			"It takes the key from the agent only when the agent connects from that address. %s", host, setIt)}
}

// sshHostKeyNeeded reports whether a login with no host key would fail
// strict host-key checking: checking is on and no known_hosts file exists.
func sshHostKeyNeeded(cfg *terminal.SSHConfig) bool {
	return cfg != nil && strings.TrimSpace(cfg.HostKey) == "" &&
		(cfg.StrictHostKey || !shared.InsecureSSHHostKeysAllowed()) &&
		len(shared.DiscoverKnownHostsFiles()) == 0
}

func (d *Deps) assetAgentVersion(assetID string) string {
	if d.AssetStore == nil {
		return ""
	}
	asset, ok, err := d.AssetStore.GetAsset(assetID)
	if err != nil || !ok {
		return ""
	}
	return strings.TrimSpace(asset.Metadata["agent_version"])
}

// agentVersionBefore reports whether version (such as "v1.6.2") is a release
// before target. A version it cannot read, such as a development build, is
// not called older.
func agentVersionBefore(version string, target [3]int) bool {
	core := strings.TrimPrefix(strings.TrimSpace(version), "v")
	if cut := strings.IndexAny(core, "-+ "); cut >= 0 {
		core = core[:cut]
	}
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return false
	}
	for index, part := range parts {
		number, err := strconv.Atoi(part)
		if err != nil || number < 0 {
			return false
		}
		if number != target[index] {
			return number < target[index]
		}
	}
	return false
}

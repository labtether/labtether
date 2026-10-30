package terminal

import (
	"fmt"
	"strconv"
	"strings"
)

// firstAgentVersionWithSSHHostKeys is the first Go agent release that reports
// the device's SSH host keys when it installs the Hub SSH key.
var firstAgentVersionWithSSHHostKeys = [3]int{1, 7, 1}

type savedSSHHostKeyMissingError struct{ text string }

func (e savedSSHHostKeyMissingError) Error() string { return e.text }
func (e savedSSHHostKeyMissingError) Unwrap() error { return errSavedSSHHostKeyMissing }

// savedSSHHostKeyMissingError states what the Hub knows about why the saved
// login has no host key: an agent version that cannot report it, or otherwise
// that the key is unknown and where to set it.
func (d *Deps) savedSSHHostKeyMissingError(assetID, host string) error {
	if version := d.assetAgentVersion(assetID); version != "" && agentVersionBefore(version, firstAgentVersionWithSSHHostKeys) {
		return savedSSHHostKeyMissingError{text: fmt.Sprintf(
			"The agent on this device is version %s, which does not report the device's SSH host key, so the Hub cannot verify the device. "+
				"Update the agent to 1.7.1 or later; the Hub takes the key when the agent reconnects.", version)}
	}
	return savedSSHHostKeyMissingError{text: fmt.Sprintf(
		"The Hub does not know this device's SSH host key for %s, so it cannot verify the device. "+
			"It takes the key from the agent only when the agent connects from that address. "+
			"To set it yourself, add SSH in the device's Connect panel with the address and host key.", host)}
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

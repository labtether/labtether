package terminal

import (
	"errors"
	"fmt"
	"strings"

	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/terminal"
)

// savedSSHUserMissingMessage is shown in the terminal when the saved SSH login
// for a device has no user, for example after an agent running as a Windows
// service installed the Hub SSH key.
const savedSSHUserMissingMessage = "This device has no SSH user. Open its Connect panel, add SSH with a user name, and choose the Hub SSH key as the credential."

var errSavedSSHUserMissing = errors.New(savedSSHUserMissingMessage)

// errSavedSSHHostKeyMissing marks a saved login with no host key while the
// Hub has no known_hosts file, so it cannot verify the device. The text shown
// comes from savedSSHHostKeyMissingError.
var errSavedSSHHostKeyMissing = errors.New("saved ssh login has no host key")

// resolveSavedTerminalSSH uses the device's saved SSH login, which the Hub
// stores when an agent installs the Hub SSH key. It returns nil, nil when no
// login is saved. Host-key checking is the same as for other SSH logins.
func (d *Deps) resolveSavedTerminalSSH(assetID string) (*terminal.SSHConfig, error) {
	if d.CredentialStore == nil || d.GetAssetTerminalConfig == nil {
		return nil, nil
	}
	cfg, ok, err := d.GetAssetTerminalConfig(assetID)
	if err != nil {
		return nil, fmt.Errorf("failed to look up saved ssh login: %w", err)
	}
	if !ok {
		return nil, nil
	}
	cfg.Username = shared.SSHLoginUser(cfg.Username)
	if profileID := strings.TrimSpace(cfg.CredentialProfileID); profileID != "" {
		profile, found, err := d.CredentialStore.GetCredentialProfile(profileID)
		if err != nil {
			return nil, fmt.Errorf("failed to load credential profile: %w", err)
		}
		if found && strings.TrimSpace(profile.Kind) == credentials.KindHubSSHIdentity &&
			(d.HubSSHKeyEnabled == nil || !d.HubSSHKeyEnabled(assetID)) {
			return nil, shared.ErrHubSSHKeyOff
		}
		if found && cfg.Username == "" {
			cfg.Username = shared.SSHLoginUser(profile.Username)
		}
	}
	if cfg.Username == "" {
		return nil, errSavedSSHUserMissing
	}
	resolved, err := d.ResolveAssetTerminalConfig(cfg)
	if err != nil {
		return nil, err
	}
	if sshHostKeyNeeded(resolved) {
		return nil, d.sshHostKeyMissingError(assetID, resolved.Host, false)
	}
	return resolved, nil
}

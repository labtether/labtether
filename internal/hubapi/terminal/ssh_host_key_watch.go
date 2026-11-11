package terminal

import (
	"errors"
	"net"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"

	"github.com/labtether/labtether/internal/terminal"
)

// hostKeyWatch notes the key a server showed when the saved host key refused
// it, so the terminal can say the key changed and show the new fingerprint.
// The refusal happens during key exchange, before any login is sent.
type hostKeyWatch struct {
	mu      sync.Mutex
	refused ssh.PublicKey
}

// watchHostKey wraps cfg's host key check when resolved has a saved key.
func watchHostKey(cfg *ssh.ClientConfig, resolved *terminal.SSHConfig) *hostKeyWatch {
	watch := &hostKeyWatch{}
	if cfg == nil || cfg.HostKeyCallback == nil || resolved == nil || strings.TrimSpace(resolved.HostKey) == "" {
		return watch
	}
	inner := cfg.HostKeyCallback
	cfg.HostKeyCallback = func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		err := inner(hostname, remote, key)
		if err != nil {
			watch.mu.Lock()
			watch.refused = key
			watch.mu.Unlock()
		}
		return err
	}
	return watch
}

// changedMessage is the sentence for a refused key, if one was refused.
func (w *hostKeyWatch) changedMessage(resolved *terminal.SSHConfig) (string, bool) {
	w.mu.Lock()
	refused := w.refused
	w.mu.Unlock()
	if refused == nil || resolved == nil {
		return "", false
	}
	saved := strings.TrimSpace(resolved.HostKey)
	savedText := hostKeyFingerprint(saved)
	if savedText == "" && strings.HasPrefix(saved, "SHA256:") {
		savedText = saved
	}
	if savedText == "" {
		savedText = "another key"
	}
	return hostKeyChangedMessage(resolved.Host, resolved.Port, savedText, ssh.FingerprintSHA256(refused)), true
}

// sshSetupErrorStage and sshSetupErrorText show a host key or bookmark login
// problem in its own words (no prefix); other setup errors keep the prefix.
func sshSetupErrorStage(err error) string {
	var unknown errHostKeyUnknown
	if errors.As(err, &unknown) {
		return "ssh_host_key_unknown"
	}
	return "ssh_config_invalid"
}

func sshSetupErrorText(prefix string, err error) string {
	var unknown errHostKeyUnknown
	if errors.As(err, &unknown) || errors.Is(err, errBookmarkNoLogin) || errors.Is(err, errSavedSSHHostKeyMissing) {
		return err.Error()
	}
	return prefix + err.Error()
}

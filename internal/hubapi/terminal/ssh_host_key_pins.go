package terminal

import (
	"context"
	"errors"
	"net"
	"strconv"
	"strings"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/labtether/labtether/internal/fileproto"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/terminal"
)

// Host key states a check reports to the console.
const (
	hostKeyTrusted   = "trusted"   // the server shows the key LabTether saved (or known_hosts has it)
	hostKeyUnknown   = "unknown"   // nothing saved yet: show the fingerprint, ask to trust it
	hostKeyChanged   = "changed"   // the server shows another key than the saved one: refused
	hostKeyUnchecked = "unchecked" // checking is off for this connection and the Hub allows that
)

// hostKeyCheck is what a check found for one host and port.
type hostKeyCheck struct {
	Status           string `json:"status"`
	Host             string `json:"host"`
	Port             int    `json:"port"`
	HostKey          string `json:"host_key,omitempty"`
	Fingerprint      string `json:"fingerprint,omitempty"`
	KeyType          string `json:"key_type,omitempty"`
	SavedFingerprint string `json:"saved_fingerprint,omitempty"`
	BookmarkID       string `json:"bookmark_id,omitempty"`
}

// knownHostKey returns the key trusted for host:port through Quick connect.
func (d *Deps) knownHostKey(host string, port int) (string, error) {
	if d.SSHKnownHostStore == nil {
		return "", nil
	}
	known, ok, err := d.SSHKnownHostStore.GetSSHKnownHost(host, port)
	if err != nil || !ok {
		return "", err
	}
	return strings.TrimSpace(known.HostKey), nil
}

func (d *Deps) probeSSHHostKey(ctx context.Context, host string, port int) (string, string, error) {
	if d.ProbeSSHHostKey != nil {
		return d.ProbeSSHHostKey(ctx, host, port)
	}
	return fileproto.DiscoverSSHHostKey(ctx, host, port)
}

// checkHostKey reads the key host:port shows now (a connection that stops at
// the host key, so no login is sent) and compares it with saved, the key
// trusted before ("" for none). Without a saved key, a key the Hub's
// known_hosts files accept counts as trusted.
func (d *Deps) checkHostKey(ctx context.Context, host string, port int, saved string) (hostKeyCheck, error) {
	result := hostKeyCheck{Host: host, Port: port}
	shown, fingerprint, err := d.probeSSHHostKey(ctx, host, port)
	if err != nil {
		return result, err
	}
	key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(shown))
	if err != nil {
		return result, err
	}
	result.HostKey, result.Fingerprint, result.KeyType = shown, fingerprint, key.Type()
	switch {
	case saved == "":
		result.Status = hostKeyUnknown
		accepted, listed := knownHostsCheck(host, port, key)
		if accepted {
			result.Status = hostKeyTrusted
		} else if listed != "" {
			// The Hub's known_hosts file lists another key: never offer to
			// trust over it.
			result.Status, result.SavedFingerprint = hostKeyChanged, listed
		}
	case sameHostKey(saved, key):
		result.Status = hostKeyTrusted
	default:
		result.Status = hostKeyChanged
		result.SavedFingerprint = hostKeyFingerprint(saved)
	}
	return result, nil
}

// knownHostsCheck reports whether the Hub's known_hosts files list key for
// host:port, and otherwise the fingerprint of another key they list for it.
func knownHostsCheck(host string, port int, key ssh.PublicKey) (bool, string) {
	callback, err := shared.BuildKnownHostsHostKeyCallback()
	if err != nil {
		return false, ""
	}
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	err = callback(addr, &net.TCPAddr{IP: net.ParseIP(host), Port: port}, key)
	if err == nil {
		return true, ""
	}
	var keyErr *knownhosts.KeyError
	if errors.As(err, &keyErr) && len(keyErr.Want) > 0 {
		return false, ssh.FingerprintSHA256(keyErr.Want[0].Key)
	}
	return false, ""
}

func knownHostsListsOther(host string, port int, key ssh.PublicKey) bool {
	accepted, listed := knownHostsCheck(host, port, key)
	return !accepted && listed != ""
}

// needsTrustedHostKey reports whether a strict connection to a host with no
// saved key would be refused: the Hub has no known_hosts file to check it.
func needsTrustedHostKey(saved string) bool {
	return strings.TrimSpace(saved) == "" && len(shared.DiscoverKnownHostsFiles()) == 0
}

func sameHostKey(saved string, key ssh.PublicKey) bool {
	parsed, _, _, _, err := ssh.ParseAuthorizedKey([]byte(strings.TrimSpace(saved)))
	return err == nil && parsed.Type() == key.Type() && string(parsed.Marshal()) == string(key.Marshal())
}

func hostKeyFingerprint(authorizedKey string) string {
	parsed, _, _, _, err := ssh.ParseAuthorizedKey([]byte(strings.TrimSpace(authorizedKey)))
	if err != nil {
		return ""
	}
	return ssh.FingerprintSHA256(parsed)
}

// bookmarkHostTarget is the host and port a bookmark to a host (not a
// device) connects to.
func bookmarkHostTarget(bookmark terminal.Bookmark) (string, int, bool) {
	if strings.TrimSpace(bookmark.AssetID) != "" || strings.TrimSpace(bookmark.Host) == "" {
		return "", 0, false
	}
	port := 22
	if bookmark.Port != nil && *bookmark.Port > 0 {
		port = *bookmark.Port
	}
	return strings.TrimSpace(bookmark.Host), port, true
}

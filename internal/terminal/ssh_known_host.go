package terminal

import (
	"net"
	"strconv"
	"strings"
	"time"
)

// SSHKnownHost is an SSH host key a person trusted on first connect (Quick
// connect), kept by the Hub for one host and port.
type SSHKnownHost struct {
	Host        string    `json:"host"`
	Port        int       `json:"port"`
	HostKey     string    `json:"host_key"`
	Fingerprint string    `json:"fingerprint"`
	PinnedBy    string    `json:"pinned_by,omitempty"`
	PinnedAt    time.Time `json:"pinned_at"`
}

// SSHPinHost is the form a host is pinned under: as typed, without case,
// IPv6 brackets or a trailing dot. Port 0 means 22.
func SSHPinHost(host string, port int) (string, int) {
	host = strings.ToLower(strings.TrimSuffix(strings.Trim(strings.TrimSpace(host), "[]"), "."))
	if port <= 0 {
		port = 22
	}
	return host, port
}

// SSHPinTarget is "host:port" in pinned form, the key a bookmark's pin is
// stored for.
func SSHPinTarget(host string, port int) string {
	host, port = SSHPinHost(host, port)
	if host == "" {
		return ""
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}

// PinnedHostKey returns the bookmark's trusted host key while the bookmark
// still points to the host and port it was trusted for, and "" otherwise.
func (b Bookmark) PinnedHostKey() string {
	key := strings.TrimSpace(b.HostKey)
	if key == "" || strings.TrimSpace(b.AssetID) != "" {
		return ""
	}
	port := 22
	if b.Port != nil {
		port = *b.Port
	}
	target := SSHPinTarget(b.Host, port)
	if target == "" || target != strings.TrimSpace(b.HostKeyTarget) {
		return ""
	}
	return key
}

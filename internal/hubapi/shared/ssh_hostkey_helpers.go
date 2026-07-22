package shared

import (
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

const EnvAllowInsecureSSHHostKeys = "LABTETHER_ALLOW_INSECURE_SSH_HOST_KEYS"

const (
	EnvSSHKnownHostsPath  = "SSH_KNOWN_HOSTS_PATH"
	EnvSSHKnownHostsPaths = "SSH_KNOWN_HOSTS_PATHS"
)

// InsecureSSHHostKeysAllowed is the single, explicit escape hatch for
// disabling SSH server identity verification. A per-asset false setting is
// not sufficient by itself, which prevents zero-value configuration from
// silently downgrading every SSH connection.
func InsecureSSHHostKeysAllowed() bool {
	return EnvOrDefaultBool(EnvAllowInsecureSSHHostKeys, false)
}

// BuildSSHHostKeyCallback applies the process-wide fail-closed SSH identity
// policy consistently. Non-strict mode requires both the caller's explicit
// opt-out and LABTETHER_ALLOW_INSECURE_SSH_HOST_KEYS=true. Expected keys may
// be OpenSSH authorized-key text, SHA256 fingerprints, or raw base64 keys.
func BuildSSHHostKeyCallback(strict bool, expected string) (ssh.HostKeyCallback, error) {
	expected = strings.TrimSpace(expected)
	if !strict && InsecureSSHHostKeysAllowed() {
		// nosemgrep: go.lang.security.audit.crypto.insecure_ssh.avoid-ssh-insecure-ignore-host-key -- reachable only after both per-connection opt-out and explicit process-wide acknowledgement.
		return ssh.InsecureIgnoreHostKey(), nil // #nosec G106 -- guarded by both caller intent and a process-wide acknowledgement.
	}
	if expected == "" {
		return BuildKnownHostsHostKeyCallback()
	}
	if publicKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(expected)); err == nil {
		return ssh.FixedHostKey(publicKey), nil
	}
	return func(_ string, _ net.Addr, key ssh.PublicKey) error {
		fingerprint := strings.TrimSpace(ssh.FingerprintSHA256(key))
		if strings.EqualFold(fingerprint, expected) {
			return nil
		}
		encoded := base64.StdEncoding.EncodeToString(key.Marshal())
		if strings.EqualFold(encoded, expected) {
			return nil
		}
		return fmt.Errorf("host key mismatch")
	}, nil
}

func BuildKnownHostsHostKeyCallback() (ssh.HostKeyCallback, error) {
	paths := DiscoverKnownHostsFiles()
	if len(paths) == 0 {
		return nil, fmt.Errorf("no known_hosts files found")
	}
	callback, err := knownhosts.New(paths...)
	if err != nil {
		return nil, fmt.Errorf("load known_hosts callback: %w", err)
	}
	return callback, nil
}

func DiscoverKnownHostsFiles() []string {
	paths := make([]string, 0, 4)
	seen := make(map[string]struct{}, 4)
	add := func(raw string) {
		path := strings.TrimSpace(raw)
		if path == "" {
			return
		}
		clean := filepath.Clean(path)
		if _, ok := seen[clean]; ok {
			return
		}
		if info, err := os.Stat(clean); err == nil && !info.IsDir() {
			paths = append(paths, clean)
			seen[clean] = struct{}{}
		}
	}

	customPathsConfigured := false
	for _, key := range []string{EnvSSHKnownHostsPaths, EnvSSHKnownHostsPath} {
		raw, configured := os.LookupEnv(key)
		customPathsConfigured = customPathsConfigured || configured
		for _, path := range strings.Split(raw, ",") {
			add(path)
		}
	}

	// Explicit path configuration is authoritative, including an explicitly
	// empty value. This lets operators and tests fail closed without silently
	// inheriting host identity material from the runtime image.
	if !customPathsConfigured {
		if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
			add(filepath.Join(home, ".ssh", "known_hosts"))
		}
		add("/etc/ssh/ssh_known_hosts")
	}

	return paths
}

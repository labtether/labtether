package shared

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

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

	for _, raw := range strings.Split(EnvOrDefault("SSH_KNOWN_HOSTS_PATHS", ""), ",") {
		add(raw)
	}
	for _, raw := range strings.Split(EnvOrDefault("SSH_KNOWN_HOSTS_PATH", ""), ",") {
		add(raw)
	}

	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		add(filepath.Join(home, ".ssh", "known_hosts"))
	}
	add("/etc/ssh/ssh_known_hosts")

	return paths
}

package remoteaccess

import (
	"os"
	"path/filepath"
	"strings"
)

var DiscoverDisplayXAuthorityFn = DiscoverDisplayXAuthority
var X11ProcRoot = "/proc"

func DiscoverDisplayXAuthority(display string) string {
	display = NormalizeX11DisplayIdentifier(display)
	if display == "" {
		return ""
	}

	if path := DiscoverDisplayXAuthorityFromProc(display); path != "" {
		return path
	}

	for _, candidate := range DisplayXAuthorityCandidates(display) {
		if candidate == "" {
			continue
		}
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}

	return ""
}

func DiscoverDisplayXAuthorityFromProc(display string) string {
	entries, err := os.ReadDir(X11ProcRoot)
	if err != nil {
		return ""
	}

	for _, entry := range entries {
		if !entry.IsDir() || !IsNumericString(entry.Name()) {
			continue
		}
		args, err := ReadProcCmdline(filepath.Join(X11ProcRoot, entry.Name(), "cmdline"))
		if err != nil || len(args) == 0 {
			continue
		}
		if !LooksLikeXServerProcess(args[0]) || !ArgsContainExactValue(args, display) {
			continue
		}
		if authPath := ExtractXAuthorityArg(args); authPath != "" {
			if info, statErr := os.Stat(authPath); statErr == nil && !info.IsDir() {
				return authPath
			}
		}
	}

	return ""
}

func DisplayXAuthorityCandidates(display string) []string {
	return []string{
		filepath.Join("/run/lightdm/root", display),
		filepath.Join("/var/run/lightdm/root", display),
		"/var/lib/lightdm/.Xauthority",
		"/var/lib/gdm3/.Xauthority",
		"/run/gdm3/auth-for-gdm-X/database",
	}
}

func ReadProcCmdline(path string) ([]string, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- Path is a procfs/X11 auth file discovered from bounded local probes.
	if err != nil {
		return nil, err
	}
	parts := strings.Split(string(data), "\x00")
	args := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		args = append(args, part)
	}
	return args, nil
}

func LooksLikeXServerProcess(command string) bool {
	base := strings.ToLower(filepath.Base(strings.TrimSpace(command)))
	return strings.Contains(base, "xorg") || strings.Contains(base, "xwayland")
}

func ArgsContainExactValue(args []string, want string) bool {
	for _, arg := range args {
		if strings.TrimSpace(arg) == want {
			return true
		}
	}
	return false
}

func ExtractXAuthorityArg(args []string) string {
	for i := 0; i < len(args)-1; i++ {
		if strings.TrimSpace(args[i]) != "-auth" {
			continue
		}
		return strings.TrimSpace(args[i+1])
	}
	return ""
}

func IsNumericString(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

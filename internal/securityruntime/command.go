package securityruntime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

const (
	envExecAllowlistMode        = "LABTETHER_EXEC_ALLOWLIST_MODE"
	envExecAllowedBinaries      = "LABTETHER_EXEC_ALLOWED_BINARIES"
	envExecBlockedBinaries      = "LABTETHER_EXEC_BLOCKED_BINARIES"
	envShellAllowlistMode       = "LABTETHER_SHELL_COMMAND_ALLOWLIST_MODE"
	envShellAllowlistPrefixes   = "LABTETHER_SHELL_COMMAND_ALLOWLIST_PREFIXES"
	envShellBlockedSubstrings   = "LABTETHER_SHELL_COMMAND_BLOCKED_SUBSTRINGS"
	defaultShellCommandFallback = "command is required"
)

var defaultAllowedExecBinaries = []string{
	"apk",
	"apt",
	"apt-get",
	"ash",
	"bash",
	"brew",
	"cat",
	"cmd",
	"dash",
	"defaults",
	"df",
	"dnf",
	"docker",
	"docker-compose",
	"dpkg-query",
	"dscl",
	"du",
	"ffmpeg",
	"free",
	"grep",
	"gst-inspect-1.0",
	"gst-launch-1.0",
	"head",
	"ifconfig",
	"ip",
	"istats",
	"journalctl",
	"launchctl",
	"loginctl",
	"log",
	"ls",
	"lscpu",
	"netplan",
	"netstat",
	"networksetup",
	"needs-restarting",
	"nmcli",
	"osascript",
	"osx-cpu-temp",
	"pacman",
	"pbcopy",
	"pbpaste",
	"ping",
	"plutil",
	"powershell",
	"ps",
	"pwsh",
	"route",
	"rpm",
	"sensors",
	"sh",
	"sysctl",
	"system_profiler",
	"systemctl",
	"tailscale",
	"tail",
	"tmux",
	"top",
	"uname",
	"uptime",
	"vncserver",
	"tvnserver",
	"winvnc4",
	"who",
	"xvfb",
	"x11vnc",
	"xsetroot",
	"xterm",
	"yum",
	"xclip",
	"xdotool",
	"ydotool",
	"xrandr",
	"xsel",
	"xset",
	"zsh",
	"zypper",
}

var defaultShellAllowlistPrefixes = []string{
	"uptime",
	"uname",
	"df",
	"du",
	"free",
	"ps",
	"top",
	"journalctl",
	"systemctl status",
	"docker ps",
	"docker images",
	"ls",
	"cat",
	"grep",
	"tail",
	"head",
}

// defaultShellBlockedSubstrings lists patterns that are blocked via substring
// matching after lowercasing and whitespace normalization. Use these for
// patterns where substring presence is inherently dangerous regardless of
// surrounding context (e.g. "rm -rf /" can never be benign as a substring).
var defaultShellBlockedSubstrings = []string{
	"rm -rf /",
	":(){ :|:& };:",
	"mkfs",
	// Catch systemctl power-state transitions not covered by token blocking.
	"systemctl poweroff",
	"systemctl reboot",
	"systemctl halt",
}

// defaultShellBlockedTokens lists command words that are blocked only when
// they appear as a standalone word token (not as a substring of another word).
// This prevents false positives: "shutdown" blocks "shutdown now" but not
// "cat shutdown.log". "reboot" blocks "reboot" but not "needs-reboot-check".
var defaultShellBlockedTokens = []string{
	"shutdown",
	"reboot",
	"halt",
	"poweroff",
	"init",
}

var runtimeEnvOverridesState struct {
	sync.RWMutex
	values map[string]string
}

func SetRuntimeEnvOverrides(values map[string]string) {
	cloned := make(map[string]string, len(values))
	for rawKey, rawValue := range values {
		key := strings.TrimSpace(rawKey)
		if key == "" {
			continue
		}
		cloned[key] = strings.TrimSpace(rawValue)
	}

	runtimeEnvOverridesState.Lock()
	defer runtimeEnvOverridesState.Unlock()
	if len(cloned) == 0 {
		runtimeEnvOverridesState.values = nil
		return
	}
	runtimeEnvOverridesState.values = cloned
}

func lookupEnv(key string) (string, bool) {
	runtimeEnvOverridesState.RLock()
	if runtimeEnvOverridesState.values != nil {
		if value, ok := runtimeEnvOverridesState.values[key]; ok {
			runtimeEnvOverridesState.RUnlock()
			return value, true
		}
	}
	runtimeEnvOverridesState.RUnlock()
	return os.LookupEnv(key)
}

func parseBoolEnv(key string, fallback bool) bool {
	raw, _ := lookupEnv(key)
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return value
}

func parseBoolEnvWithPresence(key string, fallback bool) (bool, bool) {
	raw, present := lookupEnv(key)
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback, false
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback, present
	}
	return value, present
}

func parseCSVEnv(key string, fallback []string) []string {
	raw, _ := lookupEnv(key)
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return append([]string(nil), fallback...)
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		normalized := strings.TrimSpace(part)
		if normalized == "" {
			continue
		}
		out = append(out, normalized)
	}
	if len(out) == 0 {
		return append([]string(nil), fallback...)
	}
	return out
}

func normalizeExecutableName(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return ""
	}
	trimmed = strings.ReplaceAll(trimmed, "\\", "/")
	trimmed = filepath.Base(trimmed)
	trimmed = strings.TrimSpace(trimmed)
	if trimmed == "" {
		return ""
	}
	return strings.ToLower(trimmed)
}

func normalizeShellCommand(raw string) string {
	parts := strings.Fields(strings.ToLower(strings.TrimSpace(raw)))
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " ")
}

func toSet(values []string, normalize func(string) string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		key := normalize(value)
		if key == "" {
			continue
		}
		out[key] = struct{}{}
	}
	return out
}

func ValidateExecBinary(name string) error {
	normalized := normalizeExecutableName(name)
	if normalized == "" {
		return fmt.Errorf("command is required")
	}

	blocked := toSet(parseCSVEnv(envExecBlockedBinaries, nil), normalizeExecutableName)
	if _, found := blocked[normalized]; found {
		return fmt.Errorf("command %q blocked by runtime policy", normalized)
	}

	if !parseBoolEnv(envExecAllowlistMode, true) {
		return nil
	}

	allowed := toSet(parseCSVEnv(envExecAllowedBinaries, defaultAllowedExecBinaries), normalizeExecutableName)
	if _, found := allowed[normalized]; found {
		return nil
	}

	return fmt.Errorf("command %q is not allowlisted", normalized)
}

// containsCommandToken returns true if any word in the normalized command
// exactly matches the given token. This prevents "shutdown" from matching
// "cat shutdown.log" while still catching "sudo shutdown -h now".
func containsCommandToken(normalizedCmd, token string) bool {
	for _, word := range strings.Fields(normalizedCmd) {
		if word == token {
			return true
		}
	}
	return false
}

func ValidateShellCommand(command string) error {
	normalized := normalizeShellCommand(command)
	if normalized == "" {
		return errors.New(defaultShellCommandFallback)
	}

	for _, blocked := range parseCSVEnv(envShellBlockedSubstrings, defaultShellBlockedSubstrings) {
		token := normalizeShellCommand(blocked)
		if token == "" {
			continue
		}
		if strings.Contains(normalized, token) {
			return fmt.Errorf("command blocked by safety policy")
		}
	}

	// Check blocked tokens: matched as whole words to avoid false positives
	// (e.g. "shutdown" blocks "shutdown now" but not "cat shutdown.log").
	for _, tok := range defaultShellBlockedTokens {
		normTok := strings.ToLower(strings.TrimSpace(tok))
		if normTok == "" {
			continue
		}
		if containsCommandToken(normalized, normTok) {
			return fmt.Errorf("command blocked by safety policy")
		}
	}

	if !parseBoolEnv(envShellAllowlistMode, true) {
		return nil
	}

	for _, prefix := range parseCSVEnv(envShellAllowlistPrefixes, defaultShellAllowlistPrefixes) {
		normalizedPrefix := normalizeShellCommand(prefix)
		if normalizedPrefix == "" {
			continue
		}
		if strings.HasPrefix(normalized, normalizedPrefix) {
			return nil
		}
	}

	return fmt.Errorf("command not in allowlist")
}

func NewCommandContext(ctx context.Context, name string, args ...string) (*exec.Cmd, error) {
	if err := ValidateExecBinary(name); err != nil {
		return nil, err
	}
	// #nosec G204 -- command name is validated by ValidateExecBinary allowlist/policy.
	return exec.CommandContext(ctx, name, args...), nil
}

func NewCommand(name string, args ...string) (*exec.Cmd, error) {
	if err := ValidateExecBinary(name); err != nil {
		return nil, err
	}
	// #nosec G204 -- command name is validated by ValidateExecBinary allowlist/policy.
	return exec.Command(name, args...), nil
}

func CommandContextCombinedOutput(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd, err := NewCommandContext(ctx, name, args...)
	if err != nil {
		return nil, err
	}
	return cmd.CombinedOutput()
}

func CommandCombinedOutput(name string, args ...string) ([]byte, error) {
	cmd, err := NewCommand(name, args...)
	if err != nil {
		return nil, err
	}
	return cmd.CombinedOutput()
}

func CommandContextOutput(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd, err := NewCommandContext(ctx, name, args...)
	if err != nil {
		return nil, err
	}
	return cmd.Output()
}

func CommandOutput(name string, args ...string) ([]byte, error) {
	cmd, err := NewCommand(name, args...)
	if err != nil {
		return nil, err
	}
	return cmd.Output()
}

func CommandRun(name string, args ...string) error {
	cmd, err := NewCommand(name, args...)
	if err != nil {
		return err
	}
	return cmd.Run()
}

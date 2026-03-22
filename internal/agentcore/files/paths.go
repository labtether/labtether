package files

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

// DesktopSessionInfo carries the minimal desktop session fields needed for
// home directory resolution. This mirrors the subset of the root agentcore
// desktopSessionInfo that file path resolution requires.
type DesktopSessionInfo struct {
	Username string
	UID      int
}

// DetectDesktopSessionFn is called to discover the active desktop session.
// Root agentcore wires this to its own detectDesktopSession at init time.
var DetectDesktopSessionFn = func() DesktopSessionInfo { return DesktopSessionInfo{} }

var (
	FileUserHomeDirFn          = os.UserHomeDir
	FileLookupUserByUsernameFn = user.Lookup
	FileLookupUserByUIDFn      = user.LookupId
	FileIsWritableDirFn        = IsWritableDirectory
	FileTempDirFn              = os.TempDir
	FileGetwdFn                = os.Getwd
)

// ResolveFileBaseDir resolves the file base directory for the given mode.
func ResolveFileBaseDir(fileRootMode string) string {
	return ResolveFileBaseDirWithHome(fileRootMode, ResolveAgentFileHomeDir())
}

// ResolveFileBaseDirWithHome resolves the file base directory given a mode and pre-resolved home.
func ResolveFileBaseDirWithHome(fileRootMode, homeDir string) string {
	mode := strings.TrimSpace(strings.ToLower(fileRootMode))
	home := strings.TrimSpace(homeDir)
	if home == "" {
		home = string(filepath.Separator)
	}
	if mode == "full" {
		return FilesystemRootForPath(home)
	}
	if home != "" {
		return filepath.Clean(home)
	}
	if cwd, err := FileGetwdFn(); err == nil && strings.TrimSpace(cwd) != "" {
		return filepath.Clean(cwd)
	}
	return string(filepath.Separator)
}

// ResolveAgentFileHomeDir resolves the agent's effective home directory.
func ResolveAgentFileHomeDir() string {
	if home := strings.TrimSpace(resolveProcessUserHomeDir()); home != "" && FileIsWritableDirFn(home) {
		return filepath.Clean(home)
	}

	session := DetectDesktopSessionFn()
	if home := strings.TrimSpace(resolveDesktopSessionHomeDir(session)); home != "" && FileIsWritableDirFn(home) {
		return filepath.Clean(home)
	}

	stagingHome := filepath.Join(FileTempDirFn(), "labtether-agent-home")
	if err := os.MkdirAll(stagingHome, 0o750); err == nil && FileIsWritableDirFn(stagingHome) {
		return filepath.Clean(stagingHome)
	}

	if cwd, err := FileGetwdFn(); err == nil && strings.TrimSpace(cwd) != "" && FileIsWritableDirFn(cwd) {
		return filepath.Clean(cwd)
	}

	if home := strings.TrimSpace(resolveProcessUserHomeDir()); home != "" {
		return filepath.Clean(home)
	}

	return string(filepath.Separator)
}

func resolveProcessUserHomeDir() string {
	home, err := FileUserHomeDirFn()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(home)
}

func resolveDesktopSessionHomeDir(session DesktopSessionInfo) string {
	if username := strings.TrimSpace(session.Username); username != "" {
		if usr, err := FileLookupUserByUsernameFn(username); err == nil {
			if home := strings.TrimSpace(usr.HomeDir); home != "" {
				return home
			}
		}
	}
	if session.UID > 0 {
		if usr, err := FileLookupUserByUIDFn(fmt.Sprintf("%d", session.UID)); err == nil {
			if home := strings.TrimSpace(usr.HomeDir); home != "" {
				return home
			}
		}
	}
	return ""
}

// IsWritableDirectory checks whether path is an existing writable directory.
func IsWritableDirectory(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return false
	}
	probe, err := os.CreateTemp(path, ".labtether-write-check-*")
	if err != nil {
		return false
	}
	probePath := probe.Name()
	_ = probe.Close()
	_ = os.Remove(probePath) // #nosec G703 -- Temp probe path comes from os.CreateTemp and is package-controlled.
	return true
}

// FilesystemRootForPath returns the filesystem root for a given path.
func FilesystemRootForPath(path string) string {
	cleaned := filepath.Clean(strings.TrimSpace(path))
	volume := filepath.VolumeName(cleaned)
	if volume != "" {
		return volume + string(filepath.Separator)
	}
	return string(filepath.Separator)
}

// ValidatePath resolves and validates a path, preventing traversal attacks.
// The resolved path must remain under BaseDir.
func (fm *Manager) ValidatePath(rawPath string) (string, error) {
	if rawPath == "" {
		rawPath = fm.BaseDir
	}

	// Expand ~ to home directory.
	if strings.HasPrefix(rawPath, "~/") || rawPath == "~" {
		home := strings.TrimSpace(fm.HomeDir)
		if home == "" {
			home = strings.TrimSpace(ResolveAgentFileHomeDir())
		}
		if home == "" {
			return "", fmt.Errorf("cannot resolve home directory")
		}
		rawPath = filepath.Join(home, strings.TrimPrefix(rawPath, "~"))
	}

	// Make absolute.
	if !filepath.IsAbs(rawPath) {
		rawPath = filepath.Join(fm.BaseDir, rawPath)
	}

	cleaned := filepath.Clean(rawPath)

	// Resolve baseDir symlinks first so path containment checks are performed on
	// canonical paths.
	resolvedBase, err := filepath.EvalSymlinks(filepath.Clean(fm.BaseDir))
	if err != nil {
		resolvedBase = filepath.Clean(fm.BaseDir)
	}

	// Resolve full target path when it exists. This blocks symlink escapes where
	// the final path component points outside baseDir.
	resolved := cleaned
	if info, statErr := os.Lstat(cleaned); statErr == nil && info != nil {
		resolvedTarget, evalErr := filepath.EvalSymlinks(cleaned)
		if evalErr != nil {
			return "", fmt.Errorf("resolve path %q: %w", cleaned, evalErr)
		}
		resolved = resolvedTarget
	} else if statErr == nil {
		// Should never happen, but keep a deterministic fallback.
		resolved = cleaned
	} else if errors.Is(statErr, os.ErrNotExist) {
		// For new targets, resolve parent symlinks and join basename.
		parent := filepath.Dir(cleaned)
		resolvedParent, evalErr := filepath.EvalSymlinks(parent)
		if evalErr != nil {
			resolvedParent = filepath.Clean(parent)
		}
		resolved = filepath.Join(resolvedParent, filepath.Base(cleaned))
	} else {
		return "", fmt.Errorf("stat path %q: %w", cleaned, statErr)
	}

	if !PathWithinBaseDir(resolvedBase, resolved) {
		return "", fmt.Errorf("path %q is outside the allowed base directory", rawPath)
	}

	return resolved, nil
}

// PathWithinBaseDir checks whether path is within baseDir.
func PathWithinBaseDir(baseDir, path string) bool {
	baseDir = filepath.Clean(baseDir)
	path = filepath.Clean(path)

	rel, err := filepath.Rel(baseDir, path)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	if rel == ".." {
		return false
	}
	return !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

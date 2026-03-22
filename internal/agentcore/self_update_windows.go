//go:build windows

package agentcore

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func replaceExecutable(executablePath string, binary []byte) error {
	info, err := os.Stat(executablePath)
	if err != nil {
		return err
	}
	mode := info.Mode().Perm()
	if mode == 0 {
		mode = 0o755
	}

	dir := filepath.Dir(executablePath)
	base := filepath.Base(executablePath)

	// On Windows, a running .exe cannot be overwritten directly.
	// Use rename-dance: write new → rename current to .old → rename new to current → delete .old.
	newPath := filepath.Join(dir, fmt.Sprintf("%s.new-%d", base, time.Now().UnixNano()))
	oldPath := filepath.Join(dir, fmt.Sprintf("%s.old-%d", base, time.Now().UnixNano()))

	if err := os.WriteFile(newPath, binary, mode); err != nil {
		return fmt.Errorf("write new binary: %w", err)
	}

	if err := os.Rename(executablePath, oldPath); err != nil {
		_ = os.Remove(newPath)
		return fmt.Errorf("rename current to old: %w", err)
	}

	if err := os.Rename(newPath, executablePath); err != nil {
		_ = os.Rename(oldPath, executablePath) // try to restore
		_ = os.Remove(newPath)
		return fmt.Errorf("rename new to current: %w", err)
	}

	// Best-effort cleanup. May fail if old process still holds the file.
	_ = os.Remove(oldPath)

	return nil
}

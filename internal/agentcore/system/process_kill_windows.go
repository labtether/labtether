//go:build windows

package system

import (
	"fmt"
	"os"
)

// KillProcess terminates a process on Windows.
// Windows does not support Unix signals; only forced termination (SIGKILL) is
// available. All signal values are accepted but behave identically -- the process
// is forcefully terminated via os.Process.Kill().
func KillProcess(pid int, signal string) error {
	if signal != "" && signal != "SIGKILL" && signal != "SIGTERM" && signal != "SIGINT" && signal != "SIGHUP" {
		return fmt.Errorf("unsupported signal: %q", signal)
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find pid %d: %w", pid, err)
	}

	if err := proc.Kill(); err != nil {
		return fmt.Errorf("kill pid %d: %w", pid, err)
	}
	return nil
}

//go:build linux || darwin || freebsd

package system

import (
	"fmt"
	"syscall"
)

// KillProcess sends a Unix signal to the process identified by pid.
// Supported signal names: "SIGTERM" (default), "SIGKILL", "SIGINT", "SIGHUP".
func KillProcess(pid int, signal string) error {
	var sig syscall.Signal
	switch signal {
	case "SIGKILL":
		sig = syscall.SIGKILL
	case "SIGINT":
		sig = syscall.SIGINT
	case "SIGHUP":
		sig = syscall.SIGHUP
	case "SIGTERM", "":
		sig = syscall.SIGTERM
	default:
		return fmt.Errorf("unsupported signal: %q", signal)
	}

	if err := syscall.Kill(pid, sig); err != nil {
		return fmt.Errorf("kill pid %d signal %s: %w", pid, sig, err)
	}
	return nil
}

//go:build !windows

package windows

import (
	"fmt"

	"github.com/labtether/labtether/internal/agentcore"
)

// IsWindowsService always returns false on non-Windows platforms.
func IsWindowsService() bool {
	return false
}

// RunAsService is a no-op on non-Windows platforms.
func RunAsService(_ agentcore.RuntimeConfig, _ agentcore.TelemetryProvider) error {
	return fmt.Errorf("Windows Service support is not available on this platform")
}

// InstallService is a no-op on non-Windows platforms.
func InstallService() error {
	return fmt.Errorf("Windows Service install is not available on this platform")
}

// UninstallService is a no-op on non-Windows platforms.
func UninstallService() error {
	return fmt.Errorf("Windows Service uninstall is not available on this platform")
}

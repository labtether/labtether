package backends

import (
	"fmt"
	"runtime"

	"github.com/labtether/labtether/internal/agentmgr"
)

// PackageActionResult holds the output and reboot status of a package action.
type PackageActionResult struct {
	Output         string
	RebootRequired bool
}

// PackageBackend is the platform abstraction for querying and managing packages.
type PackageBackend interface {
	ListPackages() ([]agentmgr.PackageInfo, error)
	PerformAction(action string, packages []string) (PackageActionResult, error)
}

// NewPackageBackendForOS returns the package backend appropriate for the current OS.
func NewPackageBackendForOS() PackageBackend {
	return NewPackageBackend(runtime.GOOS)
}

// NewPackageBackend returns the package backend for the given GOOS value.
func NewPackageBackend(goos string) PackageBackend {
	switch goos {
	case "linux":
		return LinuxPackageBackend{}
	case "darwin":
		return newDarwinPackageBackend()
	case "windows":
		return WindowsPackageBackend{backend: "winget"}
	default:
		return UnsupportedPackageBackend{OS: goos}
	}
}

// UnsupportedPackageBackend is the fallback backend for platforms without package support.
type UnsupportedPackageBackend struct {
	OS string
}

// ListPackages returns an error indicating the platform is unsupported.
func (b UnsupportedPackageBackend) ListPackages() ([]agentmgr.PackageInfo, error) {
	return nil, fmt.Errorf("package listing is not supported on %s", b.OS)
}

// PerformAction returns an error indicating the platform is unsupported.
func (b UnsupportedPackageBackend) PerformAction(_ string, _ []string) (PackageActionResult, error) {
	return PackageActionResult{}, fmt.Errorf("package actions are not supported on %s", b.OS)
}

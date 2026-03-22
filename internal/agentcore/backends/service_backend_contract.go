package backends

import (
	"fmt"
	"runtime"

	"github.com/labtether/labtether/internal/agentmgr"
)

// ServiceBackend is the platform abstraction for querying and managing services.
type ServiceBackend interface {
	ListServices() ([]agentmgr.ServiceInfo, error)
	PerformAction(action, service string) (string, error)
}

// NewServiceBackendForOS returns the service backend appropriate for the current OS.
func NewServiceBackendForOS() ServiceBackend {
	return NewServiceBackend(runtime.GOOS)
}

// NewServiceBackend returns the service backend for the given GOOS value.
func NewServiceBackend(goos string) ServiceBackend {
	switch goos {
	case "linux":
		return LinuxServiceBackend{}
	case "darwin":
		return DarwinServiceBackend{}
	case "windows":
		return WindowsServiceBackend{}
	default:
		return UnsupportedServiceBackend{OS: goos}
	}
}

// UnsupportedServiceBackend is the fallback backend for platforms without service support.
type UnsupportedServiceBackend struct {
	OS string
}

// ListServices returns an error indicating the platform is unsupported.
func (b UnsupportedServiceBackend) ListServices() ([]agentmgr.ServiceInfo, error) {
	return nil, fmt.Errorf("service listing is not supported on %s", b.OS)
}

// PerformAction returns an error indicating the platform is unsupported.
func (b UnsupportedServiceBackend) PerformAction(_, _ string) (string, error) {
	return "", fmt.Errorf("service actions are not supported on %s", b.OS)
}

package backends

import (
	"fmt"
	"runtime"

	"github.com/labtether/labtether/internal/agentmgr"
)

// CronBackend is the platform abstraction for querying cron/timer entries.
type CronBackend interface {
	ListEntries() ([]agentmgr.CronEntry, error)
}

// NewCronBackendForOS returns the cron backend appropriate for the current OS.
func NewCronBackendForOS() CronBackend {
	return NewCronBackend(runtime.GOOS)
}

// NewCronBackend returns the cron backend for the given GOOS value.
func NewCronBackend(goos string) CronBackend {
	switch goos {
	case "linux":
		return LinuxCronBackend{}
	case "darwin":
		return DarwinCronBackend{}
	case "windows":
		return WindowsCronBackend{}
	default:
		return UnsupportedCronBackend{OS: goos}
	}
}

// UnsupportedCronBackend is the fallback backend for platforms without cron support.
type UnsupportedCronBackend struct {
	OS string
}

// ListEntries returns an error indicating the platform is unsupported.
func (b UnsupportedCronBackend) ListEntries() ([]agentmgr.CronEntry, error) {
	return nil, fmt.Errorf("schedule listing is not supported on %s", b.OS)
}

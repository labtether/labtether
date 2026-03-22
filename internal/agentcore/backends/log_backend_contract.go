package backends

import (
	"context"
	"errors"
	"fmt"
	"runtime"

	"github.com/labtether/labtether/internal/agentmgr"
)

// ErrLogStreamingUnsupported is returned by backends that do not support log streaming.
var ErrLogStreamingUnsupported = errors.New("log streaming is not supported on this host")

// LogBackend is the platform abstraction for querying and streaming system logs.
type LogBackend interface {
	QueryEntries(req agentmgr.JournalQueryData) ([]agentmgr.LogStreamData, error)
	StreamEntries(ctx context.Context, emit func(agentmgr.LogStreamData)) error
}

// NewLogBackendForOS returns the log backend appropriate for the current OS.
func NewLogBackendForOS() LogBackend {
	return NewLogBackend(runtime.GOOS)
}

// NewLogBackend returns the log backend for the given GOOS value.
func NewLogBackend(goos string) LogBackend {
	switch goos {
	case "linux":
		return LinuxLogBackend{}
	case "darwin":
		return newDarwinLogBackend()
	case "windows":
		return WindowsLogBackend{}
	default:
		return UnsupportedLogBackend{OS: goos}
	}
}

// UnsupportedLogBackend is the fallback backend for platforms without log support.
type UnsupportedLogBackend struct {
	OS string
}

// QueryEntries returns an error indicating the platform is unsupported.
func (b UnsupportedLogBackend) QueryEntries(_ agentmgr.JournalQueryData) ([]agentmgr.LogStreamData, error) {
	return nil, fmt.Errorf("historical log queries are not supported on %s", b.OS)
}

// StreamEntries returns ErrLogStreamingUnsupported.
func (b UnsupportedLogBackend) StreamEntries(_ context.Context, _ func(agentmgr.LogStreamData)) error {
	return ErrLogStreamingUnsupported
}

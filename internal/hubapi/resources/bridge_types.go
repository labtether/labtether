package resources

import (
	"strings"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/hubapi/shared"
)

// Bridge types — exported so cmd/labtether can alias them.

// FileBridge holds channels for a pending file operation.
type FileBridge struct {
	Ch              chan agentmgr.Message
	Done            chan struct{}
	ExpectedAssetID string
}

// NewFileBridge creates a new file bridge with the given buffer size.
func NewFileBridge(buffer int, expectedAssetID string) *FileBridge {
	if buffer <= 0 {
		buffer = 1
	}
	return &FileBridge{
		Ch:              make(chan agentmgr.Message, buffer),
		Done:            make(chan struct{}),
		ExpectedAssetID: strings.TrimSpace(expectedAssetID),
	}
}

// Close closes the done channel safely.
func (b *FileBridge) Close() {
	if b == nil || b.Done == nil {
		return
	}
	select {
	case <-b.Done:
	default:
		close(b.Done)
	}
}

// ProcessBridge holds the channel for a pending process list request.
type ProcessBridge struct {
	Ch              chan agentmgr.Message
	ExpectedAssetID string
}

// ServiceBridge holds the channel for a pending service request.
type ServiceBridge struct {
	Ch              chan agentmgr.Message
	ExpectedAssetID string
}

// JournalBridge holds the channel for a pending journal query.
type JournalBridge struct {
	Ch              chan agentmgr.Message
	ExpectedAssetID string
}

// DiskBridge holds the channel for a pending disk list request.
type DiskBridge struct {
	Ch              chan agentmgr.Message
	ExpectedAssetID string
}

// NetworkBridge holds the channel for a pending network request.
type NetworkBridge struct {
	Ch              chan agentmgr.Message
	ExpectedAssetID string
}

// PackageBridge holds the channel for a pending package request.
type PackageBridge struct {
	Ch              chan agentmgr.Message
	ExpectedAssetID string
}

// CronBridge holds the channel for a pending cron list request.
type CronBridge struct {
	Ch              chan agentmgr.Message
	ExpectedAssetID string
}

// UsersBridge holds the channel for a pending users list request.
type UsersBridge struct {
	Ch              chan agentmgr.Message
	ExpectedAssetID string
}

// GenerateRequestID returns a unique request ID.
func GenerateRequestID() string { return shared.GenerateRequestID() }

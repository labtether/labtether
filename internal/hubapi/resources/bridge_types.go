package resources

import (
	"strings"
	"sync"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/hubapi/shared"
)

// Bridge types — exported so cmd/labtether can alias them.

// FileBridge holds channels for a pending file operation.
type FileBridge struct {
	Ch              chan agentmgr.Message
	Done            chan struct{}
	ExpectedAssetID string

	mu  sync.Mutex
	err error
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
	b.mu.Lock()
	defer b.mu.Unlock()
	select {
	case <-b.Done:
	default:
		close(b.Done)
	}
}

func (b *FileBridge) Fail(err error) {
	if b == nil || err == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.err == nil {
		b.err = err
	}
	select {
	case <-b.Done:
	default:
		close(b.Done)
	}
}

func (b *FileBridge) Err() error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.err
}

// GenerateRequestID returns a unique request ID.
func GenerateRequestID() string { return shared.GenerateRequestID() }

package backends

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
)

const (
	// LogBatchMaxEntries is the maximum number of entries per log batch.
	LogBatchMaxEntries = 50
	logBatchMaxAge     = 2 * time.Second
)

// MessageSender abstracts the agent-to-hub send capability so this package
// does not depend on the concrete wsTransport type in the parent agentcore package.
type MessageSender interface {
	Send(msg agentmgr.Message) error
	Connected() bool
	AssetID() string
}

// LogManager tails the system journal and forwards log entries to the hub
// via MsgLogBatch messages. It runs as a fire-and-forget producer goroutine
// independent of the receiveLoop.
type LogManager struct {
	Backend LogBackend
	mu      sync.Mutex
	cancel  context.CancelFunc
	running bool
}

// NewLogManager creates a LogManager ready to start.
func NewLogManager() *LogManager {
	return &LogManager{
		Backend: NewLogBackendForOS(),
	}
}

// CloseAll stops the journal tail goroutine if it is running.
func (lm *LogManager) CloseAll() {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	if lm.running && lm.cancel != nil {
		lm.cancel()
		lm.running = false
	}
}

// Start launches the journalctl tail in a background goroutine. It silently
// returns without error when log streaming is not available on this platform.
// The goroutine exits cleanly when ctx is cancelled or the backend stream stops.
func (lm *LogManager) Start(ctx context.Context, transport MessageSender) {
	lm.mu.Lock()
	if lm.running {
		lm.mu.Unlock()
		return
	}
	tailCtx, cancel := context.WithCancel(ctx)
	lm.cancel = cancel
	lm.running = true
	lm.mu.Unlock()

	go func() {
		defer func() {
			lm.mu.Lock()
			lm.running = false
			lm.mu.Unlock()
			cancel()
		}()

		backoff := time.Second

		for {
			var (
				batch   = make([]agentmgr.LogStreamData, 0, LogBatchMaxEntries)
				flushAt = time.Now().Add(logBatchMaxAge)
			)

			flush := func() {
				if len(batch) == 0 {
					return
				}
				if transport == nil || !transport.Connected() {
					batch = batch[:0]
					flushAt = time.Now().Add(logBatchMaxAge)
					return
				}
				data, err := json.Marshal(agentmgr.LogBatchData{
					AssetID: transport.AssetID(),
					Entries: batch,
				})
				if err != nil {
					log.Printf("logmgr: failed to marshal log batch: %v", err)
					batch = batch[:0]
					flushAt = time.Now().Add(logBatchMaxAge)
					return
				}
				_ = transport.Send(agentmgr.Message{
					Type: agentmgr.MsgLogBatch,
					Data: data,
				})
				batch = batch[:0]
				flushAt = time.Now().Add(logBatchMaxAge)
			}

			streamErr := lm.Backend.StreamEntries(tailCtx, func(entry agentmgr.LogStreamData) {
				if transport == nil || !transport.Connected() {
					return
				}
				batch = append(batch, entry)
				if len(batch) >= LogBatchMaxEntries || time.Now().After(flushAt) {
					flush()
				}
			})
			flush()

			if tailCtx.Err() != nil {
				return
			}
			if errors.Is(streamErr, ErrLogStreamingUnsupported) {
				return
			}
			// Only restart on actual errors (e.g. journalctl crash). A nil
			// return means the stream completed normally — nothing to retry.
			if streamErr == nil {
				return
			}
			log.Printf("logmgr: stream backend error (restarting in %v): %v", backoff, streamErr)

			select {
			case <-tailCtx.Done():
				return
			case <-time.After(backoff):
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
		}
	}()
}

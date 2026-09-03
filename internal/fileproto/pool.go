package fileproto

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const defaultIdleTimeout = 5 * time.Minute
const connectionGateCount = 64

type poolEntry struct {
	fs       RemoteFS
	config   ConnectionConfig
	lastUsed time.Time
}

// Pool manages reusable RemoteFS sessions keyed by connection ID.
type Pool struct {
	mu       sync.Mutex
	sessions map[string]*poolEntry
	gates    [connectionGateCount]sync.RWMutex
	newFS    func(string) (RemoteFS, error)
	done     chan struct{}
}

func NewPool() *Pool {
	p := &Pool{
		sessions: make(map[string]*poolEntry),
		newFS:    newRemoteFS,
		done:     make(chan struct{}),
	}
	go p.reapLoop()
	return p
}

// Get returns an existing session or creates a new one.
// If the cached session's connection has died, it is transparently reconnected.
func (p *Pool) Get(ctx context.Context, connectionID string, config ConnectionConfig) (RemoteFS, error) {
	gateID := config.ConnectionID
	if gateID == "" {
		gateID = connectionID
	}
	gate := p.connectionGate(gateID)
	gate.RLock()
	defer gate.RUnlock()

	if config.ValidateCurrent != nil {
		if err := config.ValidateCurrent(ctx); err != nil {
			return nil, fmt.Errorf("validate current connection: %w", err)
		}
	}

	p.mu.Lock()
	entry, ok := p.sessions[connectionID]
	if ok {
		entry.lastUsed = time.Now()
	}
	p.mu.Unlock()

	if ok {
		// Health check: try a cheap operation to verify the connection is alive.
		initialPath := config.InitialPath
		if initialPath == "" {
			initialPath = "/"
		}
		if _, err := entry.fs.List(ctx, initialPath, false); err == nil {
			return entry.fs, nil
		}
		// Stale connection — re-acquire lock and verify it's still our entry
		// before closing (another goroutine or reaper may have already replaced it).
		p.mu.Lock()
		if current, exists := p.sessions[connectionID]; exists && current == entry {
			delete(p.sessions, connectionID)
		}
		p.mu.Unlock()
		// Close outside the lock to avoid blocking other pool operations.
		closeAndLog("close stale pooled session", entry.fs.Close)
	}

	fs, err := p.newFS(config.Protocol)
	if err != nil {
		return nil, err
	}
	if err := fs.Connect(ctx, config); err != nil {
		return nil, fmt.Errorf("connect %s: %w", config.Protocol, err)
	}

	p.mu.Lock()
	// If another goroutine raced and already inserted a session, close ours
	// and use theirs to avoid leaking the duplicate.
	if existing, ok := p.sessions[connectionID]; ok {
		existing.lastUsed = time.Now() // refresh so reaper doesn't immediately evict
		p.mu.Unlock()
		closeAndLog("close duplicate pooled session", fs.Close)
		return existing.fs, nil
	}
	p.sessions[connectionID] = &poolEntry{
		fs:       fs,
		config:   config,
		lastUsed: time.Now(),
	}
	p.mu.Unlock()
	return fs, nil
}

func (p *Pool) connectionGate(connectionID string) *sync.RWMutex {
	// Fixed stripes avoid keeping one lock forever for every temporary test ID.
	var hash uint32 = 2166136261
	for index := 0; index < len(connectionID); index++ {
		hash ^= uint32(connectionID[index])
		hash *= 16777619
	}
	return &p.gates[hash%connectionGateCount]
}

func (p *Pool) removeEntries(connectionID string) []RemoteFS {
	p.mu.Lock()
	defer p.mu.Unlock()
	removed := make([]RemoteFS, 0, 1)
	for poolID, entry := range p.sessions {
		if poolID == connectionID || entry.config.ConnectionID == connectionID {
			removed = append(removed, entry.fs)
			delete(p.sessions, poolID)
		}
	}
	return removed
}

// BeginConnectionMutation blocks new connection attempts until the caller
// finishes changing or deleting the saved connection. Finishing always evicts
// the old session before another Get can run.
func (p *Pool) BeginConnectionMutation(connectionID string) func() {
	gate := p.connectionGate(connectionID)
	gate.Lock()
	var once sync.Once
	return func() {
		once.Do(func() {
			removed := p.removeEntries(connectionID)
			gate.Unlock()
			for _, fs := range removed {
				closeAndLog("close invalidated pooled session", fs.Close)
			}
		})
	}
}

// Remove closes and removes a session.
// Close is performed outside the lock to avoid blocking.
func (p *Pool) Remove(connectionID string) {
	finish := p.BeginConnectionMutation(connectionID)
	finish()
}

// Close shuts down the pool and all sessions.
func (p *Pool) Close() {
	// Collect all sessions under lock, then close outside to avoid blocking.
	p.mu.Lock()
	var toClose []RemoteFS
	for id, entry := range p.sessions {
		toClose = append(toClose, entry.fs)
		delete(p.sessions, id)
	}
	p.mu.Unlock()
	close(p.done)
	for _, fs := range toClose {
		closeAndLog("close pooled session during shutdown", fs.Close)
	}
}

func (p *Pool) reapLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-p.done:
			return
		case <-ticker.C:
			p.reapIdle()
		}
	}
}

func (p *Pool) reapIdle() {
	// Collect stale sessions under lock, close outside to avoid blocking.
	p.mu.Lock()
	var toClose []RemoteFS
	now := time.Now()
	for id, entry := range p.sessions {
		if now.Sub(entry.lastUsed) > defaultIdleTimeout {
			toClose = append(toClose, entry.fs)
			delete(p.sessions, id)
		}
	}
	p.mu.Unlock()
	for _, fs := range toClose {
		closeAndLog("close idle pooled session", fs.Close)
	}
}

func newRemoteFS(protocol string) (RemoteFS, error) {
	switch protocol {
	case "sftp":
		return &SFTPClient{}, nil
	case "smb":
		return &SMBClient{}, nil
	case "ftp":
		return &FTPClient{}, nil
	case "webdav":
		return &WebDAVClient{}, nil
	default:
		return nil, fmt.Errorf("unsupported protocol: %s", protocol)
	}
}

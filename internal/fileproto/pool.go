package fileproto

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const defaultIdleTimeout = 5 * time.Minute

type poolEntry struct {
	fs       RemoteFS
	config   ConnectionConfig
	lastUsed time.Time
}

// Pool manages reusable RemoteFS sessions keyed by connection ID.
type Pool struct {
	mu       sync.Mutex
	sessions map[string]*poolEntry
	done     chan struct{}
}

func NewPool() *Pool {
	p := &Pool{
		sessions: make(map[string]*poolEntry),
		done:     make(chan struct{}),
	}
	go p.reapLoop()
	return p
}

// Get returns an existing session or creates a new one.
// If the cached session's connection has died, it is transparently reconnected.
func (p *Pool) Get(ctx context.Context, connectionID string, config ConnectionConfig) (RemoteFS, error) {
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

	fs, err := newRemoteFS(config.Protocol)
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

// Remove closes and removes a session.
// Close is performed outside the lock to avoid blocking.
func (p *Pool) Remove(connectionID string) {
	p.mu.Lock()
	entry, ok := p.sessions[connectionID]
	if ok {
		delete(p.sessions, connectionID)
	}
	p.mu.Unlock()
	if ok {
		closeAndLog("close removed pooled session", entry.fs.Close)
	}
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

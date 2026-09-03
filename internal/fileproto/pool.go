package fileproto

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"
)

const defaultIdleTimeout = 5 * time.Minute
const connectionGateCount = 64

// ErrConnectionConfigChanged means a connection was changed or removed while
// an older session was still being established. Callers should reload the
// saved connection before retrying.
var ErrConnectionConfigChanged = errors.New("connection configuration changed during connect")

// ErrPoolClosed means the file session pool is shutting down.
var ErrPoolClosed = errors.New("file connection pool is closed")

type poolEntry struct {
	fs       RemoteFS
	config   ConnectionConfig
	lastUsed time.Time
}

// Pool manages reusable RemoteFS sessions keyed by connection ID.
type Pool struct {
	mu          sync.Mutex
	sessions    map[string]*poolEntry
	generations map[string]uint64
	leases      map[string]int
	nextGen     uint64
	gates       [connectionGateCount]sync.RWMutex
	newFS       func(string) (RemoteFS, error)
	done        chan struct{}
	closed      bool
}

func NewPool() *Pool {
	p := &Pool{
		sessions:    make(map[string]*poolEntry),
		generations: make(map[string]uint64),
		leases:      make(map[string]int),
		newFS:       newRemoteFS,
		done:        make(chan struct{}),
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
	generation := p.AcquireGeneration(connectionID)
	defer p.ReleaseGeneration(connectionID)
	return p.GetAtGeneration(ctx, connectionID, config, generation)
}

// AcquireGeneration returns the current invalidation generation for a connection.
// Callers that load config outside the pool should capture this before loading,
// pass it to GetAtGeneration, and defer ReleaseGeneration.
func (p *Pool) AcquireGeneration(connectionID string) uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return 0
	}
	p.leases[connectionID]++
	return p.generationLocked(connectionID)
}

// ReleaseGeneration releases a config-load lease. Unused generation state is
// discarded so failed and not-found requests cannot grow the pool forever.
func (p *Pool) ReleaseGeneration(connectionID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.leases[connectionID] <= 1 {
		delete(p.leases, connectionID)
		if _, hasSession := p.sessions[connectionID]; !hasSession {
			delete(p.generations, connectionID)
		}
		return
	}
	p.leases[connectionID]--
}

// GetAtGeneration gets a session only if the saved connection has not been
// invalidated since expectedGeneration was captured.
func (p *Pool) GetAtGeneration(ctx context.Context, connectionID string, config ConnectionConfig, expectedGeneration uint64) (RemoteFS, error) {
	for {
		var replaced *poolEntry
		p.mu.Lock()
		if p.closed {
			p.mu.Unlock()
			return nil, ErrPoolClosed
		}
		generation, generationExists := p.generations[connectionID]
		if !generationExists || generation != expectedGeneration {
			p.mu.Unlock()
			return nil, ErrConnectionConfigChanged
		}
		entry, ok := p.sessions[connectionID]
		if ok && !connectionConfigsEqual(entry.config, config) {
			delete(p.sessions, connectionID)
			replaced = entry
			ok = false
		} else if ok {
			entry.lastUsed = time.Now()
		}
		p.mu.Unlock()
		if replaced != nil {
			closeAndLog("close pooled session with changed config", replaced.fs.Close)
		}

		if ok {
			// Health check: try a cheap operation to verify the connection is alive.
			initialPath := config.InitialPath
			if initialPath == "" {
				initialPath = "/"
			}
			if _, err := entry.fs.List(ctx, initialPath, false); err == nil {
				p.mu.Lock()
				current, stillCurrent := p.sessions[connectionID]
				if stillCurrent && current == entry && p.generations[connectionID] == expectedGeneration {
					p.mu.Unlock()
					return entry.fs, nil
				}
				p.mu.Unlock()
				return nil, ErrConnectionConfigChanged
			}
			// Stale connection — verify it is still our entry before evicting it.
			p.mu.Lock()
			if current, exists := p.sessions[connectionID]; exists && current == entry {
				delete(p.sessions, connectionID)
				p.mu.Unlock()
				closeAndLog("close stale pooled session", entry.fs.Close)
			} else {
				p.mu.Unlock()
				return nil, ErrConnectionConfigChanged
			}
			continue
		}

		fs, err := p.newFS(config.Protocol)
		if err != nil {
			return nil, err
		}
		if err := fs.Connect(ctx, config); err != nil {
			return nil, fmt.Errorf("connect %s: %w", config.Protocol, err)
		}

		p.mu.Lock()
		if p.closed {
			p.mu.Unlock()
			closeAndLog("close session created after pool shutdown", fs.Close)
			return nil, ErrPoolClosed
		}
		if generation, exists := p.generations[connectionID]; !exists || generation != expectedGeneration {
			p.mu.Unlock()
			closeAndLog("close session created with stale config", fs.Close)
			return nil, ErrConnectionConfigChanged
		}
		// If another goroutine raced and inserted an equivalent session, use
		// it. Never return a session whose transport or credentials differ.
		if existing, exists := p.sessions[connectionID]; exists {
			if connectionConfigsEqual(existing.config, config) {
				existing.lastUsed = time.Now()
				p.mu.Unlock()
				closeAndLog("close duplicate pooled session", fs.Close)
				return existing.fs, nil
			}
			p.mu.Unlock()
			closeAndLog("close session that raced a config change", fs.Close)
			return nil, ErrConnectionConfigChanged
		}
		p.sessions[connectionID] = &poolEntry{
			fs:       fs,
			config:   cloneConnectionConfig(config),
			lastUsed: time.Now(),
		}
		p.mu.Unlock()
		return fs, nil
	}
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
	delete(p.generations, connectionID)
	for poolID, entry := range p.sessions {
		if poolID == connectionID || entry.config.ConnectionID == connectionID {
			removed = append(removed, entry.fs)
			delete(p.sessions, poolID)
			delete(p.generations, poolID)
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
	removed := p.removeEntries(connectionID)
	for _, fs := range removed {
		closeAndLog("close removed pooled session", fs.Close)
	}
}

// Close shuts down the pool and all sessions.
func (p *Pool) Close() {
	// Collect all sessions under lock, then close outside to avoid blocking.
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	var toClose []RemoteFS
	for id, entry := range p.sessions {
		toClose = append(toClose, entry.fs)
		delete(p.sessions, id)
		delete(p.generations, id)
	}
	clear(p.generations)
	clear(p.leases)
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
			delete(p.generations, id)
		}
	}
	p.mu.Unlock()
	for _, fs := range toClose {
		closeAndLog("close idle pooled session", fs.Close)
	}
}

func (p *Pool) generationLocked(connectionID string) uint64 {
	if generation, ok := p.generations[connectionID]; ok {
		return generation
	}
	p.nextGen++
	if p.nextGen == 0 {
		p.nextGen++
	}
	p.generations[connectionID] = p.nextGen
	return p.nextGen
}

func connectionConfigsEqual(a, b ConnectionConfig) bool {
	return reflect.DeepEqual(a, b)
}

func cloneConnectionConfig(config ConnectionConfig) ConnectionConfig {
	cloned := config
	if config.ExtraConfig != nil {
		cloned.ExtraConfig = make(map[string]any, len(config.ExtraConfig))
		for key, value := range config.ExtraConfig {
			cloned.ExtraConfig[key] = value
		}
	}
	return cloned
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

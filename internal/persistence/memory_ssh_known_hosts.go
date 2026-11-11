package persistence

import (
	"sync"
	"time"

	"github.com/labtether/labtether/internal/terminal"
)

// MemorySSHKnownHostStore is the in-process SSHKnownHostStore (tests and
// stores without a database), with the same never-replace rule as Postgres.
type MemorySSHKnownHostStore struct {
	mu    sync.Mutex
	hosts map[string]terminal.SSHKnownHost
}

func NewMemorySSHKnownHostStore() *MemorySSHKnownHostStore {
	return &MemorySSHKnownHostStore{hosts: make(map[string]terminal.SSHKnownHost)}
}

func (m *MemorySSHKnownHostStore) GetSSHKnownHost(host string, port int) (terminal.SSHKnownHost, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	known, ok := m.hosts[terminal.SSHPinTarget(host, port)]
	return known, ok, nil
}

func (m *MemorySSHKnownHostStore) AddSSHKnownHost(known terminal.SSHKnownHost) (terminal.SSHKnownHost, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	known.Host, known.Port = terminal.SSHPinHost(known.Host, known.Port)
	key := terminal.SSHPinTarget(known.Host, known.Port)
	if stored, ok := m.hosts[key]; ok {
		return stored, nil
	}
	if known.PinnedAt.IsZero() {
		known.PinnedAt = time.Now().UTC()
	}
	m.hosts[key] = known
	return known, nil
}

func (m *MemorySSHKnownHostStore) DeleteSSHKnownHost(host string, port int) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := terminal.SSHPinTarget(host, port)
	_, ok := m.hosts[key]
	delete(m.hosts, key)
	return ok, nil
}

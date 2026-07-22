package terminal

import (
	"errors"
	"strings"
	"sync"
	"time"

	terminalmodel "github.com/labtether/labtether/internal/terminal"
)

const (
	defaultEphemeralSSHConfigTTL = 8 * time.Hour
	maxEphemeralSSHConfigs       = 4096
)

var errEphemeralSSHConfigCapacity = errors.New("ephemeral SSH credential capacity reached")

type ephemeralSSHConfigEntry struct {
	config    terminalmodel.SSHConfig
	expiresAt time.Time
}

// EphemeralSSHConfigStore keeps Quick Connect credentials process-local. It is
// deliberately separate from terminal session persistence so passwords and
// private keys can never be serialized into Postgres, queue payloads, or API
// responses. Expiration is sliding and entries are also removed on session
// deletion.
type EphemeralSSHConfigStore struct {
	mu      sync.Mutex
	entries map[string]ephemeralSSHConfigEntry
	ttl     time.Duration
	now     func() time.Time
}

func NewEphemeralSSHConfigStore(ttl time.Duration) *EphemeralSSHConfigStore {
	if ttl <= 0 {
		ttl = defaultEphemeralSSHConfigTTL
	}
	return &EphemeralSSHConfigStore{
		entries: make(map[string]ephemeralSSHConfigEntry),
		ttl:     ttl,
		now:     time.Now,
	}
}

func (s *EphemeralSSHConfigStore) Put(sessionID string, config *terminalmodel.SSHConfig) error {
	if s == nil || config == nil || strings.TrimSpace(sessionID) == "" {
		return errors.New("ephemeral SSH session and config are required")
	}
	now := s.now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleteExpiredLocked(now)
	if _, exists := s.entries[sessionID]; !exists && len(s.entries) >= maxEphemeralSSHConfigs {
		return errEphemeralSSHConfigCapacity
	}
	s.entries[sessionID] = ephemeralSSHConfigEntry{config: *config, expiresAt: now.Add(s.ttl)}
	return nil
}

func (s *EphemeralSSHConfigStore) Get(sessionID string) (*terminalmodel.SSHConfig, bool) {
	if s == nil || strings.TrimSpace(sessionID) == "" {
		return nil, false
	}
	now := s.now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleteExpiredLocked(now)
	entry, ok := s.entries[sessionID]
	if !ok {
		return nil, false
	}
	entry.expiresAt = now.Add(s.ttl)
	s.entries[sessionID] = entry
	config := entry.config
	return &config, true
}

func (s *EphemeralSSHConfigStore) Delete(sessionID string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	delete(s.entries, strings.TrimSpace(sessionID))
	s.mu.Unlock()
}

func (s *EphemeralSSHConfigStore) deleteExpiredLocked(now time.Time) {
	for sessionID, entry := range s.entries {
		if !entry.expiresAt.After(now) {
			delete(s.entries, sessionID)
		}
	}
}

package persistence

import (
	"context"
	"crypto/subtle"
	"fmt"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/apikeys"
)

type memoryAPIKeyStore struct {
	mu   sync.RWMutex
	keys map[string]apikeys.APIKey
}

// NewMemoryAPIKeyStore returns an in-memory implementation of APIKeyStore.
func NewMemoryAPIKeyStore() APIKeyStore {
	return &memoryAPIKeyStore{keys: make(map[string]apikeys.APIKey)}
}

func (m *memoryAPIKeyStore) CreateAPIKey(_ context.Context, key apikeys.APIKey) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.keys[key.ID]; exists {
		return fmt.Errorf("key already exists: %s", key.ID)
	}
	m.keys[key.ID] = key
	return nil
}

func (m *memoryAPIKeyStore) LookupAPIKeyByHash(_ context.Context, secretHash string) (apikeys.APIKey, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, k := range m.keys {
		if subtle.ConstantTimeCompare([]byte(k.SecretHash), []byte(secretHash)) == 1 {
			return k, true, nil
		}
	}
	return apikeys.APIKey{}, false, nil
}

func (m *memoryAPIKeyStore) GetAPIKey(_ context.Context, id string) (apikeys.APIKey, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	k, ok := m.keys[id]
	return k, ok, nil
}

func (m *memoryAPIKeyStore) ListAPIKeys(_ context.Context) ([]apikeys.APIKey, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]apikeys.APIKey, 0, len(m.keys))
	for _, k := range m.keys {
		result = append(result, k)
	}
	return result, nil
}

func (m *memoryAPIKeyStore) UpdateAPIKey(_ context.Context, id string, name *string, scopes *[]string, allowedAssets *[]string, expiresAt *time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	k, ok := m.keys[id]
	if !ok {
		return fmt.Errorf("key not found: %s", id)
	}
	if name != nil {
		k.Name = *name
	}
	if scopes != nil {
		k.Scopes = *scopes
	}
	if allowedAssets != nil {
		k.AllowedAssets = *allowedAssets
	}
	if expiresAt != nil {
		k.ExpiresAt = expiresAt
	}
	m.keys[id] = k
	return nil
}

func (m *memoryAPIKeyStore) DeleteAPIKey(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.keys, id)
	return nil
}

func (m *memoryAPIKeyStore) TouchAPIKeyLastUsed(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	k, ok := m.keys[id]
	if !ok {
		return nil
	}
	now := time.Now().UTC()
	k.LastUsedAt = &now
	m.keys[id] = k
	return nil
}

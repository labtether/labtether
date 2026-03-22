package persistence

import (
	"context"
	"fmt"
	"sync"

	"github.com/labtether/labtether/internal/savedactions"
)

type memorySavedActionStore struct {
	mu   sync.RWMutex
	data map[string]savedactions.SavedAction
}

// NewMemorySavedActionStore returns an in-memory implementation of SavedActionStore.
func NewMemorySavedActionStore() SavedActionStore {
	return &memorySavedActionStore{data: make(map[string]savedactions.SavedAction)}
}

func (m *memorySavedActionStore) CreateSavedAction(_ context.Context, action savedactions.SavedAction) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.data[action.ID]; exists {
		return fmt.Errorf("saved action already exists: %s", action.ID)
	}
	m.data[action.ID] = action
	return nil
}

func (m *memorySavedActionStore) GetSavedAction(_ context.Context, id string) (savedactions.SavedAction, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	a, ok := m.data[id]
	return a, ok, nil
}

func (m *memorySavedActionStore) ListSavedActions(_ context.Context) ([]savedactions.SavedAction, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]savedactions.SavedAction, 0, len(m.data))
	for _, a := range m.data {
		result = append(result, a)
	}
	return result, nil
}

func (m *memorySavedActionStore) DeleteSavedAction(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, id)
	return nil
}

package persistence

import (
	"context"
	"fmt"
	"sort"
	"strings"
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

func (m *memorySavedActionStore) GetSavedAction(_ context.Context, actorID, id string) (savedactions.SavedAction, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	a, ok := m.data[id]
	if !ok || savedActionActorID(actorID) != savedActionActorID(a.CreatedBy) {
		return savedactions.SavedAction{}, false, nil
	}
	return a, ok, nil
}

func (m *memorySavedActionStore) ListSavedActions(_ context.Context, actorID string, limit, offset int) ([]savedactions.SavedAction, int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	normalizedActorID := savedActionActorID(actorID)
	result := make([]savedactions.SavedAction, 0, len(m.data))
	for _, a := range m.data {
		if savedActionActorID(a.CreatedBy) == normalizedActorID {
			result = append(result, a)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID > result[j].ID
		}
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	total := len(result)
	if offset < 0 {
		offset = 0
	}
	if offset >= total {
		return []savedactions.SavedAction{}, total, nil
	}
	if limit <= 0 {
		limit = 100
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return append([]savedactions.SavedAction(nil), result[offset:end]...), total, nil
}

func (m *memorySavedActionStore) DeleteSavedAction(_ context.Context, actorID, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.data[id]
	if !ok || savedActionActorID(actorID) != savedActionActorID(a.CreatedBy) {
		return ErrNotFound
	}
	delete(m.data, id)
	return nil
}

func savedActionActorID(actorID string) string {
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		return "system"
	}
	return actorID
}

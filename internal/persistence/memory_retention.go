package persistence

import (
	"time"

	"github.com/labtether/labtether/internal/retention"
)

func NewMemoryRetentionStore() *MemoryRetentionStore {
	return &MemoryRetentionStore{
		settings: retention.DefaultSettings(),
	}
}

func (m *MemoryRetentionStore) GetRetentionSettings() (retention.Settings, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return retention.Normalize(m.settings), nil
}

func (m *MemoryRetentionStore) SaveRetentionSettings(settings retention.Settings) (retention.Settings, error) {
	normalized := retention.Normalize(settings)
	m.mu.Lock()
	m.settings = normalized
	m.mu.Unlock()
	return normalized, nil
}

func (m *MemoryRetentionStore) PruneExpiredData(now time.Time, settings retention.Settings) (retention.PruneResult, error) {
	_ = now
	_ = settings
	return retention.PruneResult{
		RanAt: time.Now().UTC(),
	}, nil
}

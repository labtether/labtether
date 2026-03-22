package persistence

import "time"

// MemoryAdminResetStore provides an in-memory implementation of AdminResetStore for tests.
type MemoryAdminResetStore struct {
	ResetCalled bool
}

// NewMemoryAdminResetStore returns a new MemoryAdminResetStore.
func NewMemoryAdminResetStore() *MemoryAdminResetStore {
	return &MemoryAdminResetStore{}
}

// ResetAllData records that a reset was requested and returns a successful result.
func (s *MemoryAdminResetStore) ResetAllData() (AdminResetResult, error) {
	s.ResetCalled = true
	return AdminResetResult{
		TablesCleared: 38,
		ResetAt:       time.Now().UTC(),
	}, nil
}

package persistence

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/synthetic"
)

// MemorySyntheticStore provides an in-memory implementation of SyntheticStore for testing.
type MemorySyntheticStore struct {
	mu      sync.RWMutex
	checks  map[string]synthetic.Check
	results map[string][]synthetic.Result // checkID -> results (newest first)
	nextID  int
}

func NewMemorySyntheticStore() *MemorySyntheticStore {
	return &MemorySyntheticStore{
		checks:  make(map[string]synthetic.Check),
		results: make(map[string][]synthetic.Result),
	}
}

func (m *MemorySyntheticStore) CreateSyntheticCheck(req synthetic.CreateCheckRequest) (synthetic.Check, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.nextID++
	now := time.Now().UTC()
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	interval := req.IntervalSeconds
	if interval <= 0 {
		interval = 60
	}

	check := synthetic.Check{
		ID:              fmt.Sprintf("syncheck-%d", m.nextID),
		Name:            req.Name,
		CheckType:       synthetic.NormalizeCheckType(req.CheckType),
		Target:          req.Target,
		Config:          req.Config,
		IntervalSeconds: interval,
		Enabled:         enabled,
		ServiceID:       req.ServiceID,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	m.checks[check.ID] = check
	return check, nil
}

func (m *MemorySyntheticStore) GetSyntheticCheck(id string) (synthetic.Check, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	check, ok := m.checks[id]
	return check, ok, nil
}

func (m *MemorySyntheticStore) GetSyntheticCheckByServiceID(_ context.Context, serviceID string) (*synthetic.Check, error) {
	trimmed := strings.TrimSpace(serviceID)
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, check := range m.checks {
		if check.ServiceID == trimmed {
			c := check
			return &c, nil
		}
	}
	return nil, nil
}

func (m *MemorySyntheticStore) ListSyntheticChecks(limit int, enabledOnly bool) ([]synthetic.Check, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]synthetic.Check, 0, len(m.checks))
	for _, c := range m.checks {
		if enabledOnly && !c.Enabled {
			continue
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *MemorySyntheticStore) UpdateSyntheticCheck(id string, req synthetic.UpdateCheckRequest) (synthetic.Check, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	check, ok := m.checks[id]
	if !ok {
		return synthetic.Check{}, synthetic.ErrCheckNotFound
	}
	if req.Name != nil {
		check.Name = *req.Name
	}
	if req.Target != nil {
		check.Target = *req.Target
	}
	if req.Config != nil {
		check.Config = *req.Config
	}
	if req.IntervalSeconds != nil {
		check.IntervalSeconds = *req.IntervalSeconds
	}
	if req.Enabled != nil {
		check.Enabled = *req.Enabled
	}
	check.UpdatedAt = time.Now().UTC()
	m.checks[id] = check
	return check, nil
}

func (m *MemorySyntheticStore) DeleteSyntheticCheck(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.checks, id)
	delete(m.results, id)
	return nil
}

func (m *MemorySyntheticStore) RecordSyntheticResult(checkID string, result synthetic.Result) (synthetic.Result, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.nextID++
	result.ID = fmt.Sprintf("synresult-%d", m.nextID)
	result.CheckID = checkID
	if result.CheckedAt.IsZero() {
		result.CheckedAt = time.Now().UTC()
	}

	// Prepend (newest first)
	m.results[checkID] = append([]synthetic.Result{result}, m.results[checkID]...)
	return result, nil
}

func (m *MemorySyntheticStore) ListSyntheticResults(checkID string, limit int) ([]synthetic.Result, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	results := m.results[checkID]
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	out := make([]synthetic.Result, len(results))
	copy(out, results)
	return out, nil
}

func (m *MemorySyntheticStore) UpdateSyntheticCheckStatus(id string, status string, runAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	check, ok := m.checks[id]
	if !ok {
		return synthetic.ErrCheckNotFound
	}
	check.LastStatus = status
	check.LastRunAt = &runAt
	check.UpdatedAt = time.Now().UTC()
	m.checks[id] = check
	return nil
}

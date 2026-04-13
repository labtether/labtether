package persistence

import (
	"context"
	"sort"
	"sync"

	"github.com/labtether/labtether/internal/schedules"
)

type memoryScheduleStore struct {
	mu    sync.RWMutex
	tasks map[string]schedules.ScheduledTask
}

// NewMemoryScheduleStore returns an in-memory implementation of ScheduleStore.
func NewMemoryScheduleStore() ScheduleStore {
	return &memoryScheduleStore{tasks: make(map[string]schedules.ScheduledTask)}
}

func (m *memoryScheduleStore) CreateScheduledTask(_ context.Context, task schedules.ScheduledTask) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.tasks[task.ID]; exists {
		return ErrAlreadyExists
	}
	m.tasks[task.ID] = task
	return nil
}

func (m *memoryScheduleStore) GetScheduledTask(_ context.Context, id string) (schedules.ScheduledTask, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.tasks[id]
	return t, ok, nil
}

func (m *memoryScheduleStore) ListScheduledTasks(_ context.Context) ([]schedules.ScheduledTask, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]schedules.ScheduledTask, 0, len(m.tasks))
	for _, t := range m.tasks {
		result = append(result, t)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	return result, nil
}

func (m *memoryScheduleStore) UpdateScheduledTask(_ context.Context, id string, name *string, cronExpr *string, command *string, targets *[]string, groupID *string, enabled *bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tasks[id]
	if !ok {
		return ErrNotFound
	}
	if name != nil {
		t.Name = *name
	}
	if cronExpr != nil {
		t.CronExpr = *cronExpr
	}
	if command != nil {
		t.Command = *command
	}
	if targets != nil {
		t.Targets = *targets
	}
	if groupID != nil {
		t.GroupID = *groupID
	}
	if enabled != nil {
		t.Enabled = *enabled
	}
	m.tasks[id] = t
	return nil
}

func (m *memoryScheduleStore) DeleteScheduledTask(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.tasks[id]; !ok {
		return ErrNotFound
	}
	delete(m.tasks, id)
	return nil
}

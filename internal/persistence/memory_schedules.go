package persistence

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/schedules"
)

type memoryScheduleStore struct {
	mu              sync.RWMutex
	tasks           map[string]schedules.ScheduledTask
	principalCounts map[string]int
}

// NewMemoryScheduleStore returns an in-memory implementation of ScheduleStore.
func NewMemoryScheduleStore() ScheduleStore {
	return &memoryScheduleStore{
		tasks:           make(map[string]schedules.ScheduledTask),
		principalCounts: make(map[string]int),
	}
}

func (m *memoryScheduleStore) CreateScheduledTask(_ context.Context, task schedules.ScheduledTask) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.tasks[task.ID]; exists {
		return ErrAlreadyExists
	}
	if len(m.tasks) >= schedules.MaxScheduledTasksGlobal || m.principalCounts[task.CreatedBy] >= schedules.MaxScheduledTasksPerPrincipal {
		return schedules.ErrScheduledTaskCapacityExceeded
	}
	m.tasks[task.ID] = cloneScheduledTask(task)
	m.principalCounts[task.CreatedBy]++
	return nil
}

func (m *memoryScheduleStore) GetScheduledTask(_ context.Context, id string) (schedules.ScheduledTask, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.tasks[id]
	return cloneScheduledTask(t), ok, nil
}

func (m *memoryScheduleStore) ListScheduledTasks(_ context.Context, limit, offset int) ([]schedules.ScheduledTask, int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]schedules.ScheduledTask, 0, len(m.tasks))
	for _, t := range m.tasks {
		result = append(result, cloneScheduledTask(t))
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID > result[j].ID
		}
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	total := len(result)
	limit, offset = normalizeScheduledTaskListBounds(limit, offset)
	if offset >= total {
		return []schedules.ScheduledTask{}, total, nil
	}
	end := min(offset+limit, total)
	return result[offset:end], total, nil
}

func (m *memoryScheduleStore) ListScheduledTasksForEvaluation(
	ctx context.Context,
	now time.Time,
	limit int,
) ([]schedules.ScheduledTask, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	limit, _ = normalizeScheduledTaskListBounds(limit, 0)
	result := make([]schedules.ScheduledTask, 0, min(limit, len(m.tasks)))
	for _, task := range m.tasks {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !task.Enabled || (task.NextRunAt != nil && task.NextRunAt.After(now)) {
			continue
		}
		result = append(result, cloneScheduledTask(task))
	}
	sort.Slice(result, func(i, j int) bool {
		left, right := result[i].NextRunAt, result[j].NextRunAt
		if left == nil || right == nil {
			if left == nil && right == nil {
				return result[i].ID < result[j].ID
			}
			return left == nil
		}
		if left.Equal(*right) {
			return result[i].ID < result[j].ID
		}
		return left.Before(*right)
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (m *memoryScheduleStore) UpdateScheduledTask(_ context.Context, id string, name *string, cronExpr *string, command *string, targets *[]string, groupID *string, enabled *bool, nextRun schedules.NextRunUpdate) error {
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
		t.Targets = append([]string(nil), (*targets)...)
	}
	if groupID != nil {
		t.GroupID = *groupID
	}
	if enabled != nil {
		t.Enabled = *enabled
		if !*enabled {
			t.LastRunStatus = "cancelled"
			t.LastRunJobID = ""
		}
	}
	if nextRun.Set {
		t.NextRunAt = nextRun.Value
	}
	m.tasks[id] = t
	return nil
}

func normalizeScheduledTaskListBounds(limit, offset int) (int, int) {
	if limit <= 0 {
		limit = schedules.MaxScheduledTaskPageSize
	}
	if limit > schedules.MaxScheduledTasksGlobal+1 {
		limit = schedules.MaxScheduledTasksGlobal + 1
	}
	if offset < 0 {
		offset = 0
	}
	if offset > schedules.MaxScheduledTasksGlobal {
		offset = schedules.MaxScheduledTasksGlobal
	}
	return limit, offset
}

func cloneScheduledTask(task schedules.ScheduledTask) schedules.ScheduledTask {
	task.Targets = append([]string(nil), task.Targets...)
	if task.LastRunAt != nil {
		value := *task.LastRunAt
		task.LastRunAt = &value
	}
	if task.NextRunAt != nil {
		value := *task.NextRunAt
		task.NextRunAt = &value
	}
	return task
}

func (m *memoryScheduleStore) DeleteScheduledTask(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	task, ok := m.tasks[id]
	if !ok {
		return ErrNotFound
	}
	delete(m.tasks, id)
	m.principalCounts[task.CreatedBy]--
	if m.principalCounts[task.CreatedBy] <= 0 {
		delete(m.principalCounts, task.CreatedBy)
	}
	return nil
}

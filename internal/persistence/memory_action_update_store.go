package persistence

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/updates"
)

type MemoryActionStore struct {
	mu   sync.RWMutex
	runs map[string]actions.Run
}

func NewMemoryActionStore() *MemoryActionStore {
	return &MemoryActionStore{
		runs: make(map[string]actions.Run),
	}
}

func (m *MemoryActionStore) CreateActionRun(req actions.ExecuteRequest) (actions.Run, error) {
	now := time.Now().UTC()
	runType := actions.NormalizeRunType(req.Type)
	if runType == "" {
		if strings.TrimSpace(req.ConnectorID) != "" || strings.TrimSpace(req.ActionID) != "" {
			runType = actions.RunTypeConnectorAction
		} else {
			runType = actions.RunTypeCommand
		}
	}

	actorID := strings.TrimSpace(req.ActorID)
	if actorID == "" {
		actorID = "owner"
	}

	run := actions.Run{
		ID:          idgen.New("actrun"),
		Type:        runType,
		ActorID:     actorID,
		Target:      strings.TrimSpace(req.Target),
		Command:     strings.TrimSpace(req.Command),
		ConnectorID: strings.TrimSpace(req.ConnectorID),
		ActionID:    strings.TrimSpace(req.ActionID),
		Params:      cloneMetadata(req.Params),
		DryRun:      req.DryRun,
		Status:      actions.StatusQueued,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	m.mu.Lock()
	m.runs[run.ID] = run
	m.mu.Unlock()

	return cloneActionRun(run), nil
}

func (m *MemoryActionStore) GetActionRun(id string) (actions.Run, bool, error) {
	m.mu.RLock()
	run, ok := m.runs[id]
	m.mu.RUnlock()
	if !ok {
		return actions.Run{}, false, nil
	}
	return cloneActionRun(run), true, nil
}

func (m *MemoryActionStore) ListActionRuns(limit, offset int, runType, status string) ([]actions.Run, error) {
	if limit <= 0 {
		limit = 50
	}
	runType = actions.NormalizeRunType(runType)
	status = actions.NormalizeStatus(status)

	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]actions.Run, 0, len(m.runs))
	for _, run := range m.runs {
		if runType != "" && run.Type != runType {
			continue
		}
		if status != "" && run.Status != status {
			continue
		}
		out = append(out, cloneActionRun(run))
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	if offset > 0 {
		if offset >= len(out) {
			return []actions.Run{}, nil
		}
		out = out[offset:]
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *MemoryActionStore) DeleteActionRun(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	id = strings.TrimSpace(id)
	if _, ok := m.runs[id]; !ok {
		return ErrNotFound
	}
	delete(m.runs, id)
	return nil
}

func (m *MemoryActionStore) ApplyActionResult(result actions.Result) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	run, ok := m.runs[result.RunID]
	if !ok {
		return errors.New("action run not found")
	}

	status := actions.NormalizeStatus(result.Status)
	if status == "" {
		status = actions.StatusFailed
	}
	completedAt := result.CompletedAt.UTC()
	if completedAt.IsZero() {
		completedAt = time.Now().UTC()
	}

	run.Status = status
	run.Output = strings.TrimSpace(result.Output)
	run.Error = strings.TrimSpace(result.Error)
	run.UpdatedAt = completedAt
	run.CompletedAt = &completedAt
	run.Steps = make([]actions.RunStep, 0, len(result.Steps))

	for _, step := range result.Steps {
		stepStatus := actions.NormalizeStatus(step.Status)
		if stepStatus == "" {
			stepStatus = actions.StatusFailed
		}
		run.Steps = append(run.Steps, actions.RunStep{
			ID:        idgen.New("actstep"),
			RunID:     run.ID,
			Name:      strings.TrimSpace(step.Name),
			Status:    stepStatus,
			Output:    strings.TrimSpace(step.Output),
			Error:     strings.TrimSpace(step.Error),
			CreatedAt: completedAt,
			UpdatedAt: completedAt,
		})
	}

	m.runs[result.RunID] = run
	return nil
}

type MemoryUpdateStore struct {
	mu    sync.RWMutex
	plans map[string]updates.Plan
	runs  map[string]updates.Run
}

func NewMemoryUpdateStore() *MemoryUpdateStore {
	return &MemoryUpdateStore{
		plans: make(map[string]updates.Plan),
		runs:  make(map[string]updates.Run),
	}
}

func (m *MemoryUpdateStore) CreateUpdatePlan(req updates.CreatePlanRequest) (updates.Plan, error) {
	now := time.Now().UTC()
	defaultDryRun := true
	if req.DefaultDryRun != nil {
		defaultDryRun = *req.DefaultDryRun
	}

	scopes := sanitizeStringSlice(req.Scopes)
	if len(scopes) == 0 {
		scopes = append([]string(nil), updates.DefaultScopes...)
	}

	plan := updates.Plan{
		ID:            idgen.New("upln"),
		Name:          strings.TrimSpace(req.Name),
		Description:   strings.TrimSpace(req.Description),
		Targets:       sanitizeStringSlice(req.Targets),
		Scopes:        scopes,
		DefaultDryRun: defaultDryRun,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	m.mu.Lock()
	m.plans[plan.ID] = plan
	m.mu.Unlock()

	return cloneUpdatePlan(plan), nil
}

func (m *MemoryUpdateStore) ListUpdatePlans(limit int) ([]updates.Plan, error) {
	if limit <= 0 {
		limit = 50
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]updates.Plan, 0, len(m.plans))
	for _, plan := range m.plans {
		out = append(out, cloneUpdatePlan(plan))
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *MemoryUpdateStore) GetUpdatePlan(id string) (updates.Plan, bool, error) {
	m.mu.RLock()
	plan, ok := m.plans[id]
	m.mu.RUnlock()
	if !ok {
		return updates.Plan{}, false, nil
	}
	return cloneUpdatePlan(plan), true, nil
}

func (m *MemoryUpdateStore) DeleteUpdatePlan(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.plans[id]; !ok {
		return ErrNotFound
	}
	delete(m.plans, id)
	return nil
}

func (m *MemoryUpdateStore) CreateUpdateRun(plan updates.Plan, req updates.ExecutePlanRequest) (updates.Run, error) {
	now := time.Now().UTC()
	actorID := strings.TrimSpace(req.ActorID)
	if actorID == "" {
		actorID = "owner"
	}

	dryRun := plan.DefaultDryRun
	if req.DryRun != nil {
		dryRun = *req.DryRun
	}

	run := updates.Run{
		ID:        idgen.New("uprun"),
		PlanID:    plan.ID,
		PlanName:  plan.Name,
		ActorID:   actorID,
		DryRun:    dryRun,
		Status:    updates.StatusQueued,
		CreatedAt: now,
		UpdatedAt: now,
	}

	m.mu.Lock()
	m.runs[run.ID] = run
	m.mu.Unlock()
	return cloneUpdateRun(run), nil
}

func (m *MemoryUpdateStore) GetUpdateRun(id string) (updates.Run, bool, error) {
	m.mu.RLock()
	run, ok := m.runs[id]
	m.mu.RUnlock()
	if !ok {
		return updates.Run{}, false, nil
	}
	return cloneUpdateRun(run), true, nil
}

func (m *MemoryUpdateStore) ListUpdateRuns(limit int, status string) ([]updates.Run, error) {
	return m.ListUpdateRunsPage(limit, 0, status)
}

func (m *MemoryUpdateStore) ListUpdateRunsPage(limit, offset int, status string) ([]updates.Run, error) {
	if limit <= 0 {
		limit = 50
	}
	status = updates.NormalizeStatus(status)

	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]updates.Run, 0, len(m.runs))
	for _, run := range m.runs {
		if status != "" && run.Status != status {
			continue
		}
		out = append(out, cloneUpdateRun(run))
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	if offset >= len(out) {
		return []updates.Run{}, nil
	}
	out = out[offset:]
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *MemoryUpdateStore) DeleteUpdateRun(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	id = strings.TrimSpace(id)
	if _, ok := m.runs[id]; !ok {
		return ErrNotFound
	}
	delete(m.runs, id)
	return nil
}

func (m *MemoryUpdateStore) ApplyUpdateResult(result updates.Result) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	run, ok := m.runs[result.RunID]
	if !ok {
		return errors.New("update run not found")
	}

	status := updates.NormalizeStatus(result.Status)
	if status == "" {
		status = updates.StatusFailed
	}
	completedAt := result.CompletedAt.UTC()
	if completedAt.IsZero() {
		completedAt = time.Now().UTC()
	}

	run.Status = status
	run.Summary = strings.TrimSpace(result.Summary)
	run.Error = strings.TrimSpace(result.Error)
	run.Results = cloneUpdateRunResults(result.Results)
	run.UpdatedAt = completedAt
	run.CompletedAt = &completedAt

	m.runs[result.RunID] = run
	return nil
}

func cloneActionRun(input actions.Run) actions.Run {
	out := input
	out.Params = cloneMetadata(input.Params)
	if len(input.Steps) > 0 {
		out.Steps = append([]actions.RunStep(nil), input.Steps...)
	}
	return out
}

func cloneUpdatePlan(input updates.Plan) updates.Plan {
	out := input
	if len(input.Targets) > 0 {
		out.Targets = append([]string(nil), input.Targets...)
	}
	if len(input.Scopes) > 0 {
		out.Scopes = append([]string(nil), input.Scopes...)
	}
	return out
}

func cloneUpdateRun(input updates.Run) updates.Run {
	out := input
	out.Results = cloneUpdateRunResults(input.Results)
	return out
}

func cloneUpdateRunResults(input []updates.RunResultEntry) []updates.RunResultEntry {
	if len(input) == 0 {
		return nil
	}
	out := make([]updates.RunResultEntry, len(input))
	copy(out, input)
	return out
}

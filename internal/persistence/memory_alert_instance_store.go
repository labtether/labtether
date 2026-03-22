package persistence

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/idgen"
)

// MemoryAlertInstanceStore is an in-memory implementation of AlertInstanceStore.
type MemoryAlertInstanceStore struct {
	mu        sync.RWMutex
	instances map[string]alerts.AlertInstance
	silences  map[string]alerts.AlertSilence
}

// NewMemoryAlertInstanceStore returns a new in-memory alert instance store.
func NewMemoryAlertInstanceStore() *MemoryAlertInstanceStore {
	return &MemoryAlertInstanceStore{
		instances: make(map[string]alerts.AlertInstance),
		silences:  make(map[string]alerts.AlertSilence),
	}
}

func (m *MemoryAlertInstanceStore) CreateAlertInstance(req alerts.CreateInstanceRequest) (alerts.AlertInstance, error) {
	now := time.Now().UTC()
	severity := alerts.NormalizeSeverity(req.Severity)
	if severity == "" {
		severity = alerts.SeverityMedium
	}
	fingerprint := strings.TrimSpace(req.Fingerprint)
	if fingerprint == "" {
		fingerprint = alerts.GenerateFingerprint(req.RuleID, req.Labels)
	}

	inst := alerts.AlertInstance{
		ID:          idgen.New("ainst"),
		RuleID:      strings.TrimSpace(req.RuleID),
		Fingerprint: fingerprint,
		Status:      alerts.InstanceStatusPending,
		Severity:    severity,
		Labels:      cloneMetadata(req.Labels),
		Annotations: cloneMetadata(req.Annotations),
		StartedAt:   now,
		LastFiredAt: now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	m.mu.Lock()
	m.instances[inst.ID] = cloneAlertInstance(inst)
	m.mu.Unlock()
	return cloneAlertInstance(inst), nil
}

func (m *MemoryAlertInstanceStore) GetAlertInstance(id string) (alerts.AlertInstance, bool, error) {
	m.mu.RLock()
	inst, ok := m.instances[strings.TrimSpace(id)]
	m.mu.RUnlock()
	if !ok {
		return alerts.AlertInstance{}, false, nil
	}
	return cloneAlertInstance(inst), true, nil
}

func (m *MemoryAlertInstanceStore) GetActiveInstanceByFingerprint(ruleID, fingerprint string) (alerts.AlertInstance, bool, error) {
	ruleID = strings.TrimSpace(ruleID)
	fingerprint = strings.TrimSpace(fingerprint)

	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, inst := range m.instances {
		if inst.RuleID != ruleID || inst.Fingerprint != fingerprint {
			continue
		}
		if inst.Status == alerts.InstanceStatusPending ||
			inst.Status == alerts.InstanceStatusFiring ||
			inst.Status == alerts.InstanceStatusAcknowledged {
			return cloneAlertInstance(inst), true, nil
		}
	}
	return alerts.AlertInstance{}, false, nil
}

func (m *MemoryAlertInstanceStore) ListAlertInstances(filter AlertInstanceFilter) ([]alerts.AlertInstance, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	status := alerts.NormalizeInstanceStatus(filter.Status)
	severity := alerts.NormalizeSeverity(filter.Severity)
	ruleID := strings.TrimSpace(filter.RuleID)

	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]alerts.AlertInstance, 0, len(m.instances))
	for _, inst := range m.instances {
		if ruleID != "" && inst.RuleID != ruleID {
			continue
		}
		if status != "" && inst.Status != status {
			continue
		}
		if severity != "" && inst.Severity != severity {
			continue
		}
		out = append(out, cloneAlertInstance(inst))
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	if filter.Offset > 0 {
		if filter.Offset >= len(out) {
			return []alerts.AlertInstance{}, nil
		}
		out = out[filter.Offset:]
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *MemoryAlertInstanceStore) UpdateAlertInstanceStatus(id, status string) (alerts.AlertInstance, error) {
	id = strings.TrimSpace(id)
	status = alerts.NormalizeInstanceStatus(status)
	if status == "" {
		return alerts.AlertInstance{}, errors.New("invalid alert instance status")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	inst, ok := m.instances[id]
	if !ok {
		return alerts.AlertInstance{}, errors.New("alert instance not found")
	}
	if !alerts.CanTransitionInstanceStatus(inst.Status, status) {
		return alerts.AlertInstance{}, fmt.Errorf("cannot transition alert instance from %s to %s", inst.Status, status)
	}

	now := time.Now().UTC()
	inst.Status = status
	inst.UpdatedAt = now
	if status == alerts.InstanceStatusResolved {
		inst.ResolvedAt = &now
	}

	m.instances[id] = cloneAlertInstance(inst)
	return cloneAlertInstance(inst), nil
}

func (m *MemoryAlertInstanceStore) UpdateAlertInstanceLastFired(id string) error {
	id = strings.TrimSpace(id)
	now := time.Now().UTC()

	m.mu.Lock()
	defer m.mu.Unlock()

	inst, ok := m.instances[id]
	if !ok {
		return errors.New("alert instance not found")
	}
	inst.LastFiredAt = now
	inst.UpdatedAt = now
	m.instances[id] = inst
	return nil
}

func (m *MemoryAlertInstanceStore) DeleteAlertInstance(id string) error {
	id = strings.TrimSpace(id)

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.instances[id]; !ok {
		return errors.New("alert instance not found")
	}
	delete(m.instances, id)
	return nil
}

func (m *MemoryAlertInstanceStore) CreateAlertSilence(req alerts.CreateSilenceRequest) (alerts.AlertSilence, error) {
	now := time.Now().UTC()
	createdBy := strings.TrimSpace(req.CreatedBy)
	if createdBy == "" {
		createdBy = "owner"
	}

	silence := alerts.AlertSilence{
		ID:        idgen.New("sil"),
		Matchers:  cloneMetadata(req.Matchers),
		Reason:    strings.TrimSpace(req.Reason),
		CreatedBy: createdBy,
		StartsAt:  req.StartsAt.UTC(),
		EndsAt:    req.EndsAt.UTC(),
		CreatedAt: now,
	}

	m.mu.Lock()
	m.silences[silence.ID] = silence
	m.mu.Unlock()
	return silence, nil
}

func (m *MemoryAlertInstanceStore) ListAlertSilences(limit int, activeOnly bool) ([]alerts.AlertSilence, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	now := time.Now().UTC()

	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]alerts.AlertSilence, 0, len(m.silences))
	for _, silence := range m.silences {
		if activeOnly {
			if silence.StartsAt.After(now) || silence.EndsAt.Before(now) {
				continue
			}
		}
		out = append(out, silence)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *MemoryAlertInstanceStore) GetAlertSilence(id string) (alerts.AlertSilence, bool, error) {
	m.mu.RLock()
	silence, ok := m.silences[strings.TrimSpace(id)]
	m.mu.RUnlock()
	if !ok {
		return alerts.AlertSilence{}, false, nil
	}
	return silence, true, nil
}

func (m *MemoryAlertInstanceStore) DeleteAlertSilence(id string) error {
	id = strings.TrimSpace(id)

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.silences[id]; !ok {
		return errors.New("alert silence not found")
	}
	delete(m.silences, id)
	return nil
}

// BackdateStartedAt is a test helper that adjusts an instance's StartedAt
// field to simulate elapsed firing time.
func (m *MemoryAlertInstanceStore) BackdateStartedAt(id string, startedAt time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()

	inst, ok := m.instances[id]
	if !ok {
		return
	}
	inst.StartedAt = startedAt.UTC()
	m.instances[id] = inst
}

func cloneAlertInstance(input alerts.AlertInstance) alerts.AlertInstance {
	out := input
	out.Labels = cloneMetadata(input.Labels)
	out.Annotations = cloneMetadata(input.Annotations)
	if input.ResolvedAt != nil {
		value := input.ResolvedAt.UTC()
		out.ResolvedAt = &value
	}
	return out
}

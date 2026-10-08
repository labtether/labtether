package persistence

import (
	"github.com/labtether/labtether/internal/model"
	"sort"
	"strings"
	"time"
)

func (m *MemoryCanonicalModelStore) UpsertCapabilitySet(set model.CapabilitySet) (model.CapabilitySet, error) {
	now := time.Now().UTC()
	subjectType := strings.TrimSpace(strings.ToLower(set.SubjectType))
	subjectID := strings.TrimSpace(set.SubjectID)
	if subjectType == "" || subjectID == "" {
		return model.CapabilitySet{}, ErrNotFound
	}
	key := capabilitySetKey(subjectType, subjectID)

	entry := model.CapabilitySet{
		SubjectType:  subjectType,
		SubjectID:    subjectID,
		Capabilities: cloneCapabilitySpecs(set.Capabilities),
		UpdatedAt:    set.UpdatedAt.UTC(),
	}
	if entry.UpdatedAt.IsZero() {
		entry.UpdatedAt = now
	}

	m.mu.Lock()
	m.capabilitySets[key] = entry
	m.mu.Unlock()

	return cloneCapabilitySet(entry), nil
}

func (m *MemoryCanonicalModelStore) GetCapabilitySet(subjectType, subjectID string) (model.CapabilitySet, bool, error) {
	key := capabilitySetKey(subjectType, subjectID)

	m.mu.RLock()
	defer m.mu.RUnlock()

	set, ok := m.capabilitySets[key]
	if !ok {
		return model.CapabilitySet{}, false, nil
	}
	return cloneCapabilitySet(set), true, nil
}

func (m *MemoryCanonicalModelStore) ReplaceCapabilitySets(providerInstanceID string, sets []model.CapabilitySet) error {
	providerInstanceID = strings.TrimSpace(providerInstanceID)
	if providerInstanceID == "" {
		return ErrNotFound
	}

	now := time.Now().UTC()
	entries := make(map[string]model.CapabilitySet, len(sets))
	for _, set := range sets {
		subjectType := strings.TrimSpace(strings.ToLower(set.SubjectType))
		subjectID := strings.TrimSpace(set.SubjectID)
		if subjectType == "" {
			continue
		}
		if subjectType == "provider" {
			subjectID = providerInstanceID
		}
		if subjectID == "" {
			continue
		}
		entry := model.CapabilitySet{
			SubjectType:  subjectType,
			SubjectID:    subjectID,
			Capabilities: cloneCapabilitySpecs(set.Capabilities),
			UpdatedAt:    set.UpdatedAt.UTC(),
		}
		if entry.UpdatedAt.IsZero() {
			entry.UpdatedAt = now
		}
		entries[capabilitySetKey(subjectType, subjectID)] = entry
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if existingSubjects, ok := m.capabilitySubjectsByProvider[providerInstanceID]; ok {
		for key := range existingSubjects {
			delete(m.capabilitySets, key)
		}
	}

	if len(entries) == 0 {
		delete(m.capabilitySubjectsByProvider, providerInstanceID)
		return nil
	}

	subjectKeys := make(map[string]struct{}, len(entries))
	for key, entry := range entries {
		subjectKeys[key] = struct{}{}
		m.capabilitySets[key] = entry
	}
	m.capabilitySubjectsByProvider[providerInstanceID] = subjectKeys
	return nil
}

func (m *MemoryCanonicalModelStore) ListCapabilitySets(limit int) ([]model.CapabilitySet, error) {
	if limit <= 0 {
		limit = 500
	}
	if limit > 5000 {
		limit = 5000
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]model.CapabilitySet, 0, len(m.capabilitySets))
	for _, set := range m.capabilitySets {
		out = append(out, cloneCapabilitySet(set))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return capabilitySetKey(out[i].SubjectType, out[i].SubjectID) < capabilitySetKey(out[j].SubjectType, out[j].SubjectID)
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *MemoryCanonicalModelStore) UpsertTemplateBinding(binding model.TemplateBinding) (model.TemplateBinding, error) {
	resourceID := strings.TrimSpace(binding.ResourceID)
	if resourceID == "" {
		return model.TemplateBinding{}, ErrNotFound
	}
	now := time.Now().UTC()
	entry := model.TemplateBinding{
		ResourceID: resourceID,
		TemplateID: strings.TrimSpace(binding.TemplateID),
		Tabs:       cloneStrings(binding.Tabs),
		Operations: cloneStrings(binding.Operations),
		UpdatedAt:  binding.UpdatedAt.UTC(),
	}
	if entry.TemplateID == "" {
		entry.TemplateID = "template.other.default"
	}
	if entry.UpdatedAt.IsZero() {
		entry.UpdatedAt = now
	}

	m.mu.Lock()
	m.templateBindings[resourceID] = entry
	m.mu.Unlock()

	return cloneTemplateBinding(entry), nil
}

func (m *MemoryCanonicalModelStore) GetTemplateBinding(resourceID string) (model.TemplateBinding, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	binding, ok := m.templateBindings[strings.TrimSpace(resourceID)]
	if !ok {
		return model.TemplateBinding{}, false, nil
	}
	return cloneTemplateBinding(binding), true, nil
}

func (m *MemoryCanonicalModelStore) ListTemplateBindings(resourceIDs []string) ([]model.TemplateBinding, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(resourceIDs) == 0 {
		out := make([]model.TemplateBinding, 0, len(m.templateBindings))
		for _, binding := range m.templateBindings {
			out = append(out, cloneTemplateBinding(binding))
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
				return out[i].ResourceID < out[j].ResourceID
			}
			return out[i].UpdatedAt.After(out[j].UpdatedAt)
		})
		return out, nil
	}

	out := make([]model.TemplateBinding, 0, len(resourceIDs))
	for _, resourceID := range resourceIDs {
		resourceID = strings.TrimSpace(resourceID)
		if resourceID == "" {
			continue
		}
		if binding, ok := m.templateBindings[resourceID]; ok {
			out = append(out, cloneTemplateBinding(binding))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].ResourceID < out[j].ResourceID
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out, nil
}

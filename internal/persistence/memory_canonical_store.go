package persistence

import (
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/model"
)

type MemoryCanonicalModelStore struct {
	mu                              sync.RWMutex
	providers                       map[string]model.ProviderInstance
	externalRefsByResource          map[string][]model.ExternalRef
	relationshipsByProvider         map[string]map[string]model.ResourceRelationship
	capabilitySets                  map[string]model.CapabilitySet
	capabilitySubjectsByProvider    map[string]map[string]struct{}
	templateBindings                map[string]model.TemplateBinding
	ingestCheckpoints               map[string]model.IngestCheckpoint
	reconciliationResultsByProvider map[string][]model.ReconciliationResult
}

func NewMemoryCanonicalModelStore() *MemoryCanonicalModelStore {
	return &MemoryCanonicalModelStore{
		providers:                       make(map[string]model.ProviderInstance),
		externalRefsByResource:          make(map[string][]model.ExternalRef),
		relationshipsByProvider:         make(map[string]map[string]model.ResourceRelationship),
		capabilitySets:                  make(map[string]model.CapabilitySet),
		capabilitySubjectsByProvider:    make(map[string]map[string]struct{}),
		templateBindings:                make(map[string]model.TemplateBinding),
		ingestCheckpoints:               make(map[string]model.IngestCheckpoint),
		reconciliationResultsByProvider: make(map[string][]model.ReconciliationResult),
	}
}

func (m *MemoryCanonicalModelStore) UpsertProviderInstance(instance model.ProviderInstance) (model.ProviderInstance, error) {
	now := time.Now().UTC()
	id := strings.TrimSpace(instance.ID)
	if id == "" {
		return model.ProviderInstance{}, ErrNotFound
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	existing, ok := m.providers[id]
	if !ok {
		existing.CreatedAt = now
	}

	existing.ID = id
	existing.Kind = instance.Kind
	existing.Provider = strings.TrimSpace(instance.Provider)
	existing.DisplayName = strings.TrimSpace(instance.DisplayName)
	existing.Version = strings.TrimSpace(instance.Version)
	existing.Status = instance.Status
	existing.Scope = instance.Scope
	existing.ConfigRef = strings.TrimSpace(instance.ConfigRef)
	existing.Metadata = cloneAnyMap(instance.Metadata)
	existing.LastSeenAt = instance.LastSeenAt.UTC()
	if existing.LastSeenAt.IsZero() {
		existing.LastSeenAt = now
	}
	existing.UpdatedAt = now

	m.providers[id] = existing
	return cloneProviderInstance(existing), nil
}

func (m *MemoryCanonicalModelStore) GetProviderInstance(id string) (model.ProviderInstance, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	instance, ok := m.providers[strings.TrimSpace(id)]
	if !ok {
		return model.ProviderInstance{}, false, nil
	}
	return cloneProviderInstance(instance), true, nil
}

func (m *MemoryCanonicalModelStore) ListProviderInstances(limit int) ([]model.ProviderInstance, error) {
	if limit <= 0 {
		limit = 1000
	}
	if limit > 5000 {
		limit = 5000
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]model.ProviderInstance, 0, len(m.providers))
	for _, instance := range m.providers {
		out = append(out, cloneProviderInstance(instance))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *MemoryCanonicalModelStore) ReplaceResourceExternalRefs(resourceID string, refs []model.ExternalRef) error {
	resourceID = strings.TrimSpace(resourceID)
	if resourceID == "" {
		return ErrNotFound
	}

	cleaned := make([]model.ExternalRef, 0, len(refs))
	seen := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		providerID := strings.TrimSpace(ref.ProviderInstanceID)
		externalID := strings.TrimSpace(ref.ExternalID)
		if providerID == "" || externalID == "" {
			continue
		}
		key := providerID + "|" + externalID
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		cleaned = append(cleaned, model.ExternalRef{
			ProviderInstanceID: providerID,
			ExternalID:         externalID,
			ExternalType:       strings.TrimSpace(ref.ExternalType),
			ExternalParentID:   strings.TrimSpace(ref.ExternalParentID),
			RawLocator:         strings.TrimSpace(ref.RawLocator),
		})
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if len(cleaned) == 0 {
		delete(m.externalRefsByResource, resourceID)
		return nil
	}
	m.externalRefsByResource[resourceID] = cloneExternalRefs(cleaned)
	return nil
}

func (m *MemoryCanonicalModelStore) ListResourceExternalRefs(resourceID string) ([]model.ExternalRef, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	refs := m.externalRefsByResource[strings.TrimSpace(resourceID)]
	return cloneExternalRefs(refs), nil
}

func (m *MemoryCanonicalModelStore) ReplaceResourceRelationships(providerInstanceID string, relationships []model.ResourceRelationship) error {
	providerInstanceID = strings.TrimSpace(providerInstanceID)
	if providerInstanceID == "" {
		return ErrNotFound
	}

	now := time.Now().UTC()
	rels := make(map[string]model.ResourceRelationship, len(relationships))
	for _, relationship := range relationships {
		sourceID := strings.TrimSpace(relationship.SourceResourceID)
		targetID := strings.TrimSpace(relationship.TargetResourceID)
		if sourceID == "" || targetID == "" || sourceID == targetID {
			continue
		}
		id := strings.TrimSpace(relationship.ID)
		if id == "" {
			id = relationshipIdentity(sourceID, targetID, relationship.Type)
		}
		createdAt := relationship.CreatedAt.UTC()
		if createdAt.IsZero() {
			createdAt = now
		}
		rels[id] = model.ResourceRelationship{
			ID:               id,
			SourceResourceID: sourceID,
			TargetResourceID: targetID,
			Type:             relationship.Type,
			Direction:        relationship.Direction,
			Criticality:      relationship.Criticality,
			Inferred:         relationship.Inferred,
			Confidence:       relationship.Confidence,
			Evidence:         cloneAnyMap(relationship.Evidence),
			CreatedAt:        createdAt,
			UpdatedAt:        now,
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if len(rels) == 0 {
		delete(m.relationshipsByProvider, providerInstanceID)
		return nil
	}
	m.relationshipsByProvider[providerInstanceID] = rels
	return nil
}

func (m *MemoryCanonicalModelStore) ListResourceRelationships(resourceID string, limit int) ([]model.ResourceRelationship, error) {
	if limit <= 0 {
		limit = 200
	}
	if limit > 2000 {
		limit = 2000
	}
	resourceID = strings.TrimSpace(resourceID)

	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]model.ResourceRelationship, 0, 32)
	for _, byID := range m.relationshipsByProvider {
		for _, relationship := range byID {
			if resourceID != "" && relationship.SourceResourceID != resourceID && relationship.TargetResourceID != resourceID {
				continue
			}
			out = append(out, cloneResourceRelationship(relationship))
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

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

func (m *MemoryCanonicalModelStore) UpsertIngestCheckpoint(checkpoint model.IngestCheckpoint) (model.IngestCheckpoint, error) {
	providerID := strings.TrimSpace(checkpoint.ProviderInstanceID)
	stream := strings.TrimSpace(strings.ToLower(checkpoint.Stream))
	if providerID == "" || stream == "" {
		return model.IngestCheckpoint{}, ErrNotFound
	}
	entry := model.IngestCheckpoint{
		ProviderInstanceID: providerID,
		Stream:             stream,
		Cursor:             strings.TrimSpace(checkpoint.Cursor),
		SyncedAt:           checkpoint.SyncedAt.UTC(),
	}
	if entry.SyncedAt.IsZero() {
		entry.SyncedAt = time.Now().UTC()
	}

	m.mu.Lock()
	m.ingestCheckpoints[ingestCheckpointKey(providerID, stream)] = entry
	m.mu.Unlock()

	return entry, nil
}

func (m *MemoryCanonicalModelStore) GetIngestCheckpoint(providerInstanceID, stream string) (model.IngestCheckpoint, bool, error) {
	key := ingestCheckpointKey(providerInstanceID, stream)

	m.mu.RLock()
	defer m.mu.RUnlock()

	checkpoint, ok := m.ingestCheckpoints[key]
	if !ok {
		return model.IngestCheckpoint{}, false, nil
	}
	return checkpoint, true, nil
}

func (m *MemoryCanonicalModelStore) RecordReconciliationResult(result model.ReconciliationResult) (model.ReconciliationResult, error) {
	providerID := strings.TrimSpace(result.ProviderInstanceID)
	if providerID == "" {
		return model.ReconciliationResult{}, ErrNotFound
	}
	now := time.Now().UTC()
	entry := model.ReconciliationResult{
		ProviderInstanceID: providerID,
		CreatedCount:       result.CreatedCount,
		UpdatedCount:       result.UpdatedCount,
		StaleCount:         result.StaleCount,
		ErrorCount:         result.ErrorCount,
		StartedAt:          result.StartedAt.UTC(),
		FinishedAt:         result.FinishedAt.UTC(),
	}
	if entry.StartedAt.IsZero() {
		entry.StartedAt = now
	}
	if entry.FinishedAt.IsZero() {
		entry.FinishedAt = now
	}

	m.mu.Lock()
	m.reconciliationResultsByProvider[providerID] = append(
		m.reconciliationResultsByProvider[providerID],
		entry,
	)
	m.mu.Unlock()

	return entry, nil
}

func (m *MemoryCanonicalModelStore) ListReconciliationResults(providerInstanceID string, limit int) ([]model.ReconciliationResult, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 2000 {
		limit = 2000
	}
	providerInstanceID = strings.TrimSpace(providerInstanceID)

	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]model.ReconciliationResult, 0, limit)
	if providerInstanceID != "" {
		out = append(out, m.reconciliationResultsByProvider[providerInstanceID]...)
	} else {
		for _, entries := range m.reconciliationResultsByProvider {
			out = append(out, entries...)
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].FinishedAt.Equal(out[j].FinishedAt) {
			return out[i].ProviderInstanceID < out[j].ProviderInstanceID
		}
		return out[i].FinishedAt.After(out[j].FinishedAt)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func capabilitySetKey(subjectType, subjectID string) string {
	return strings.ToLower(strings.TrimSpace(subjectType)) + "|" + strings.TrimSpace(subjectID)
}

func ingestCheckpointKey(providerInstanceID, stream string) string {
	return strings.TrimSpace(providerInstanceID) + "|" + strings.ToLower(strings.TrimSpace(stream))
}

func relationshipIdentity(sourceID, targetID string, relationshipType model.RelationshipType) string {
	builder := strings.Builder{}
	builder.WriteString("rel-")
	builder.WriteString(strings.ToLower(strings.TrimSpace(sourceID)))
	builder.WriteString("-")
	builder.WriteString(string(relationshipType))
	builder.WriteString("-")
	builder.WriteString(strings.ToLower(strings.TrimSpace(targetID)))
	return builder.String()
}

func cloneProviderInstance(input model.ProviderInstance) model.ProviderInstance {
	input.Metadata = cloneAnyMap(input.Metadata)
	return input
}

func cloneExternalRefs(input []model.ExternalRef) []model.ExternalRef {
	if len(input) == 0 {
		return nil
	}
	out := make([]model.ExternalRef, len(input))
	copy(out, input)
	return out
}

func cloneResourceRelationship(input model.ResourceRelationship) model.ResourceRelationship {
	input.Evidence = cloneAnyMap(input.Evidence)
	return input
}

func cloneCapabilitySpecs(input []model.CapabilitySpec) []model.CapabilitySpec {
	if len(input) == 0 {
		return nil
	}
	out := make([]model.CapabilitySpec, len(input))
	copy(out, input)
	for idx := range out {
		out[idx].ParamsSchema = cloneAnyMap(out[idx].ParamsSchema)
	}
	return out
}

func cloneCapabilitySet(input model.CapabilitySet) model.CapabilitySet {
	input.Capabilities = cloneCapabilitySpecs(input.Capabilities)
	return input
}

func cloneTemplateBinding(input model.TemplateBinding) model.TemplateBinding {
	input.Tabs = cloneStrings(input.Tabs)
	input.Operations = cloneStrings(input.Operations)
	return input
}

func cloneStrings(input []string) []string {
	if len(input) == 0 {
		return nil
	}
	out := make([]string, len(input))
	copy(out, input)
	return out
}

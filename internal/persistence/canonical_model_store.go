package persistence

import "github.com/labtether/labtether/internal/model"

// CanonicalModelStore persists canonical abstractions that sit above source-specific payloads.
type CanonicalModelStore interface {
	UpsertProviderInstance(instance model.ProviderInstance) (model.ProviderInstance, error)
	GetProviderInstance(id string) (model.ProviderInstance, bool, error)
	ListProviderInstances(limit int) ([]model.ProviderInstance, error)

	ReplaceResourceExternalRefs(resourceID string, refs []model.ExternalRef) error
	ListResourceExternalRefs(resourceID string) ([]model.ExternalRef, error)

	ReplaceResourceRelationships(providerInstanceID string, relationships []model.ResourceRelationship) error
	ListResourceRelationships(resourceID string, limit int) ([]model.ResourceRelationship, error)

	UpsertCapabilitySet(set model.CapabilitySet) (model.CapabilitySet, error)
	GetCapabilitySet(subjectType, subjectID string) (model.CapabilitySet, bool, error)
	ReplaceCapabilitySets(providerInstanceID string, sets []model.CapabilitySet) error
	ListCapabilitySets(limit int) ([]model.CapabilitySet, error)

	UpsertTemplateBinding(binding model.TemplateBinding) (model.TemplateBinding, error)
	GetTemplateBinding(resourceID string) (model.TemplateBinding, bool, error)
	ListTemplateBindings(resourceIDs []string) ([]model.TemplateBinding, error)

	UpsertIngestCheckpoint(checkpoint model.IngestCheckpoint) (model.IngestCheckpoint, error)
	GetIngestCheckpoint(providerInstanceID, stream string) (model.IngestCheckpoint, bool, error)

	RecordReconciliationResult(result model.ReconciliationResult) (model.ReconciliationResult, error)
	ListReconciliationResults(providerInstanceID string, limit int) ([]model.ReconciliationResult, error)
}

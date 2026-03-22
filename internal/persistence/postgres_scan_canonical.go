package persistence

import (
	"encoding/json"

	"github.com/labtether/labtether/internal/model"
)

type providerInstanceScanner interface {
	Scan(dest ...any) error
}

func scanProviderInstance(row providerInstanceScanner) (model.ProviderInstance, error) {
	instance := model.ProviderInstance{}
	var version *string
	var configRef *string
	var metadata []byte

	if err := row.Scan(
		&instance.ID,
		&instance.Kind,
		&instance.Provider,
		&instance.DisplayName,
		&version,
		&instance.Status,
		&instance.Scope,
		&configRef,
		&metadata,
		&instance.LastSeenAt,
		&instance.CreatedAt,
		&instance.UpdatedAt,
	); err != nil {
		return model.ProviderInstance{}, err
	}
	if version != nil {
		instance.Version = *version
	}
	if configRef != nil {
		instance.ConfigRef = *configRef
	}
	instance.Metadata = unmarshalAnyMap(metadata)
	instance.LastSeenAt = instance.LastSeenAt.UTC()
	instance.CreatedAt = instance.CreatedAt.UTC()
	instance.UpdatedAt = instance.UpdatedAt.UTC()
	return instance, nil
}

type externalRefScanner interface {
	Scan(dest ...any) error
}

func scanExternalRef(row externalRefScanner) (model.ExternalRef, string, error) {
	ref := model.ExternalRef{}
	resourceID := ""
	var externalType *string
	var externalParentID *string
	var rawLocator *string

	if err := row.Scan(
		&resourceID,
		&ref.ProviderInstanceID,
		&ref.ExternalID,
		&externalType,
		&externalParentID,
		&rawLocator,
	); err != nil {
		return model.ExternalRef{}, "", err
	}
	if externalType != nil {
		ref.ExternalType = *externalType
	}
	if externalParentID != nil {
		ref.ExternalParentID = *externalParentID
	}
	if rawLocator != nil {
		ref.RawLocator = *rawLocator
	}
	return ref, resourceID, nil
}

type canonicalRelationshipScanner interface {
	Scan(dest ...any) error
}

func scanCanonicalRelationship(row canonicalRelationshipScanner) (model.ResourceRelationship, string, error) {
	relationship := model.ResourceRelationship{}
	providerInstanceID := ""
	var evidence []byte
	if err := row.Scan(
		&relationship.ID,
		&providerInstanceID,
		&relationship.SourceResourceID,
		&relationship.TargetResourceID,
		&relationship.Type,
		&relationship.Direction,
		&relationship.Criticality,
		&relationship.Inferred,
		&relationship.Confidence,
		&evidence,
		&relationship.CreatedAt,
		&relationship.UpdatedAt,
	); err != nil {
		return model.ResourceRelationship{}, "", err
	}
	relationship.Evidence = unmarshalAnyMap(evidence)
	relationship.CreatedAt = relationship.CreatedAt.UTC()
	relationship.UpdatedAt = relationship.UpdatedAt.UTC()
	return relationship, providerInstanceID, nil
}

type capabilitySetScanner interface {
	Scan(dest ...any) error
}

func scanCapabilitySet(row capabilitySetScanner) (model.CapabilitySet, *string, error) {
	set := model.CapabilitySet{}
	var providerInstanceID *string
	var capabilities []byte
	if err := row.Scan(
		&set.SubjectType,
		&set.SubjectID,
		&providerInstanceID,
		&capabilities,
		&set.UpdatedAt,
	); err != nil {
		return model.CapabilitySet{}, nil, err
	}
	set.UpdatedAt = set.UpdatedAt.UTC()
	if len(capabilities) > 0 {
		parsed := make([]model.CapabilitySpec, 0, 8)
		if err := json.Unmarshal(capabilities, &parsed); err == nil {
			for idx := range parsed {
				parsed[idx].ParamsSchema = cloneAnyMap(parsed[idx].ParamsSchema)
			}
			set.Capabilities = parsed
		}
	}
	return set, providerInstanceID, nil
}

type templateBindingScanner interface {
	Scan(dest ...any) error
}

func scanTemplateBinding(row templateBindingScanner) (model.TemplateBinding, error) {
	binding := model.TemplateBinding{}
	var tabs []byte
	var operations []byte
	if err := row.Scan(
		&binding.ResourceID,
		&binding.TemplateID,
		&tabs,
		&operations,
		&binding.UpdatedAt,
	); err != nil {
		return model.TemplateBinding{}, err
	}
	binding.UpdatedAt = binding.UpdatedAt.UTC()
	binding.Tabs = unmarshalStringSlice(tabs)
	binding.Operations = unmarshalStringSlice(operations)
	return binding, nil
}

type ingestCheckpointScanner interface {
	Scan(dest ...any) error
}

func scanIngestCheckpoint(row ingestCheckpointScanner) (model.IngestCheckpoint, error) {
	checkpoint := model.IngestCheckpoint{}
	var cursor *string
	if err := row.Scan(
		&checkpoint.ProviderInstanceID,
		&checkpoint.Stream,
		&cursor,
		&checkpoint.SyncedAt,
	); err != nil {
		return model.IngestCheckpoint{}, err
	}
	if cursor != nil {
		checkpoint.Cursor = *cursor
	}
	checkpoint.SyncedAt = checkpoint.SyncedAt.UTC()
	return checkpoint, nil
}

type reconciliationResultScanner interface {
	Scan(dest ...any) error
}

func scanReconciliationResult(row reconciliationResultScanner) (model.ReconciliationResult, string, error) {
	result := model.ReconciliationResult{}
	id := ""
	if err := row.Scan(
		&id,
		&result.ProviderInstanceID,
		&result.CreatedCount,
		&result.UpdatedCount,
		&result.StaleCount,
		&result.ErrorCount,
		&result.StartedAt,
		&result.FinishedAt,
	); err != nil {
		return model.ReconciliationResult{}, "", err
	}
	result.StartedAt = result.StartedAt.UTC()
	result.FinishedAt = result.FinishedAt.UTC()
	return result, id, nil
}

func marshalCapabilitySpecs(values []model.CapabilitySpec) (string, error) {
	if len(values) == 0 {
		return "[]", nil
	}
	payload, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

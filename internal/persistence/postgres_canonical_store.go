package persistence

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/model"
)

func (s *PostgresStore) UpsertProviderInstance(instance model.ProviderInstance) (model.ProviderInstance, error) {
	id := strings.TrimSpace(instance.ID)
	if id == "" {
		return model.ProviderInstance{}, ErrNotFound
	}
	now := time.Now().UTC()
	metadataPayload, err := marshalAnyMap(instance.Metadata)
	if err != nil {
		return model.ProviderInstance{}, err
	}

	lastSeenAt := instance.LastSeenAt.UTC()
	if lastSeenAt.IsZero() {
		lastSeenAt = now
	}

	provider, err := scanProviderInstance(s.pool.QueryRow(
		context.Background(),
		`INSERT INTO provider_instances (
			id, kind, provider, display_name, version, status, scope, config_ref,
			metadata, last_seen_at, created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10, $11, $11)
		ON CONFLICT (id) DO UPDATE SET
			kind = EXCLUDED.kind,
			provider = EXCLUDED.provider,
			display_name = EXCLUDED.display_name,
			version = EXCLUDED.version,
			status = EXCLUDED.status,
			scope = EXCLUDED.scope,
			config_ref = EXCLUDED.config_ref,
			metadata = EXCLUDED.metadata,
			last_seen_at = EXCLUDED.last_seen_at,
			updated_at = EXCLUDED.updated_at
		RETURNING id, kind, provider, display_name, version, status, scope, config_ref,
			metadata, last_seen_at, created_at, updated_at`,
		id,
		instance.Kind,
		strings.TrimSpace(instance.Provider),
		strings.TrimSpace(instance.DisplayName),
		nullIfBlank(instance.Version),
		instance.Status,
		instance.Scope,
		nullIfBlank(instance.ConfigRef),
		metadataPayload,
		lastSeenAt,
		now,
	))
	if err != nil {
		return model.ProviderInstance{}, err
	}
	return provider, nil
}

func (s *PostgresStore) GetProviderInstance(id string) (model.ProviderInstance, bool, error) {
	provider, err := scanProviderInstance(s.pool.QueryRow(
		context.Background(),
		`SELECT id, kind, provider, display_name, version, status, scope, config_ref,
			metadata, last_seen_at, created_at, updated_at
		 FROM provider_instances
		 WHERE id = $1`,
		strings.TrimSpace(id),
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.ProviderInstance{}, false, nil
		}
		return model.ProviderInstance{}, false, err
	}
	return provider, true, nil
}

func (s *PostgresStore) ListProviderInstances(limit int) ([]model.ProviderInstance, error) {
	if limit <= 0 {
		limit = 500
	}
	if limit > 5000 {
		limit = 5000
	}

	rows, err := s.pool.Query(
		context.Background(),
		`SELECT id, kind, provider, display_name, version, status, scope, config_ref,
			metadata, last_seen_at, created_at, updated_at
		 FROM provider_instances
		 ORDER BY updated_at DESC
		 LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]model.ProviderInstance, 0, limit)
	for rows.Next() {
		provider, scanErr := scanProviderInstance(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, provider)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (s *PostgresStore) ReplaceResourceExternalRefs(resourceID string, refs []model.ExternalRef) error {
	resourceID = strings.TrimSpace(resourceID)
	if resourceID == "" {
		return ErrNotFound
	}

	tx, err := s.pool.Begin(context.Background())
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())

	if _, err := tx.Exec(
		context.Background(),
		`DELETE FROM resource_external_refs WHERE resource_id = $1`,
		resourceID,
	); err != nil {
		return err
	}

	now := time.Now().UTC()
	for _, ref := range refs {
		providerID := strings.TrimSpace(ref.ProviderInstanceID)
		externalID := strings.TrimSpace(ref.ExternalID)
		if providerID == "" || externalID == "" {
			continue
		}
		if _, err := tx.Exec(
			context.Background(),
			`INSERT INTO resource_external_refs (
				resource_id, provider_instance_id, external_id, external_type,
				external_parent_id, raw_locator, created_at, updated_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $7)
			ON CONFLICT (resource_id, provider_instance_id, external_id) DO UPDATE SET
				external_type = EXCLUDED.external_type,
				external_parent_id = EXCLUDED.external_parent_id,
				raw_locator = EXCLUDED.raw_locator,
				updated_at = EXCLUDED.updated_at`,
			resourceID,
			providerID,
			externalID,
			nullIfBlank(ref.ExternalType),
			nullIfBlank(ref.ExternalParentID),
			nullIfBlank(ref.RawLocator),
			now,
		); err != nil {
			return err
		}
	}

	return tx.Commit(context.Background())
}

func (s *PostgresStore) ListResourceExternalRefs(resourceID string) ([]model.ExternalRef, error) {
	rows, err := s.pool.Query(
		context.Background(),
		`SELECT resource_id, provider_instance_id, external_id, external_type,
			external_parent_id, raw_locator
		 FROM resource_external_refs
		 WHERE resource_id = $1
		 ORDER BY provider_instance_id ASC, external_id ASC`,
		strings.TrimSpace(resourceID),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]model.ExternalRef, 0, 8)
	for rows.Next() {
		ref, _, scanErr := scanExternalRef(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, ref)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (s *PostgresStore) ReplaceResourceRelationships(providerInstanceID string, relationships []model.ResourceRelationship) error {
	providerInstanceID = strings.TrimSpace(providerInstanceID)
	if providerInstanceID == "" {
		return ErrNotFound
	}

	tx, err := s.pool.Begin(context.Background())
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())

	if _, err := tx.Exec(
		context.Background(),
		`DELETE FROM canonical_resource_relationships WHERE provider_instance_id = $1`,
		providerInstanceID,
	); err != nil {
		return err
	}

	now := time.Now().UTC()
	for _, relationship := range relationships {
		sourceID := strings.TrimSpace(relationship.SourceResourceID)
		targetID := strings.TrimSpace(relationship.TargetResourceID)
		if sourceID == "" || targetID == "" || sourceID == targetID {
			continue
		}
		relationshipID := strings.TrimSpace(relationship.ID)
		if relationshipID == "" {
			relationshipID = relationshipIdentity(sourceID, targetID, relationship.Type)
		}
		evidencePayload, payloadErr := marshalAnyMap(relationship.Evidence)
		if payloadErr != nil {
			return payloadErr
		}
		createdAt := relationship.CreatedAt.UTC()
		if createdAt.IsZero() {
			createdAt = now
		}
		if _, err := tx.Exec(
			context.Background(),
			`INSERT INTO canonical_resource_relationships (
				id, provider_instance_id, source_resource_id, target_resource_id,
				relationship_type, direction, criticality, inferred, confidence,
				evidence, created_at, updated_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10::jsonb, $11, $12)
			ON CONFLICT (id) DO UPDATE SET
				provider_instance_id = EXCLUDED.provider_instance_id,
				source_resource_id = EXCLUDED.source_resource_id,
				target_resource_id = EXCLUDED.target_resource_id,
				relationship_type = EXCLUDED.relationship_type,
				direction = EXCLUDED.direction,
				criticality = EXCLUDED.criticality,
				inferred = EXCLUDED.inferred,
				confidence = EXCLUDED.confidence,
				evidence = EXCLUDED.evidence,
				updated_at = EXCLUDED.updated_at`,
			relationshipID,
			providerInstanceID,
			sourceID,
			targetID,
			relationship.Type,
			relationship.Direction,
			relationship.Criticality,
			relationship.Inferred,
			relationship.Confidence,
			evidencePayload,
			createdAt,
			now,
		); err != nil {
			return err
		}
	}

	return tx.Commit(context.Background())
}

func (s *PostgresStore) ListResourceRelationships(resourceID string, limit int) ([]model.ResourceRelationship, error) {
	if limit <= 0 {
		limit = 200
	}
	if limit > 5000 {
		limit = 5000
	}
	resourceID = strings.TrimSpace(resourceID)

	var (
		rows pgx.Rows
		err  error
	)
	if resourceID == "" {
		rows, err = s.pool.Query(
			context.Background(),
			`SELECT id, provider_instance_id, source_resource_id, target_resource_id,
				relationship_type, direction, criticality, inferred, confidence,
				evidence, created_at, updated_at
			 FROM canonical_resource_relationships
			 ORDER BY updated_at DESC
			 LIMIT $1`,
			limit,
		)
	} else {
		rows, err = s.pool.Query(
			context.Background(),
			`SELECT id, provider_instance_id, source_resource_id, target_resource_id,
				relationship_type, direction, criticality, inferred, confidence,
				evidence, created_at, updated_at
			 FROM canonical_resource_relationships
			 WHERE source_resource_id = $1 OR target_resource_id = $1
			 ORDER BY updated_at DESC
			 LIMIT $2`,
			resourceID,
			limit,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]model.ResourceRelationship, 0, limit)
	for rows.Next() {
		relationship, _, scanErr := scanCanonicalRelationship(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, relationship)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (s *PostgresStore) UpsertCapabilitySet(set model.CapabilitySet) (model.CapabilitySet, error) {
	now := time.Now().UTC()
	subjectType := strings.TrimSpace(strings.ToLower(set.SubjectType))
	subjectID := strings.TrimSpace(set.SubjectID)
	if subjectType == "" || subjectID == "" {
		return model.CapabilitySet{}, ErrNotFound
	}
	providerID := ""
	if subjectType == "provider" {
		providerID = subjectID
	}
	capabilitiesPayload, err := marshalCapabilitySpecs(set.Capabilities)
	if err != nil {
		return model.CapabilitySet{}, err
	}
	updatedAt := set.UpdatedAt.UTC()
	if updatedAt.IsZero() {
		updatedAt = now
	}

	stored, _, err := scanCapabilitySet(s.pool.QueryRow(
		context.Background(),
		`INSERT INTO canonical_capability_sets (
			subject_type, subject_id, provider_instance_id, capabilities, updated_at
		)
		VALUES ($1, $2, $3, $4::jsonb, $5)
		ON CONFLICT (subject_type, subject_id) DO UPDATE SET
			provider_instance_id = COALESCE(EXCLUDED.provider_instance_id, canonical_capability_sets.provider_instance_id),
			capabilities = EXCLUDED.capabilities,
			updated_at = EXCLUDED.updated_at
		RETURNING subject_type, subject_id, provider_instance_id, capabilities, updated_at`,
		subjectType,
		subjectID,
		nullIfBlank(providerID),
		capabilitiesPayload,
		updatedAt,
	))
	if err != nil {
		return model.CapabilitySet{}, err
	}
	return stored, nil
}

func (s *PostgresStore) GetCapabilitySet(subjectType, subjectID string) (model.CapabilitySet, bool, error) {
	set, _, err := scanCapabilitySet(s.pool.QueryRow(
		context.Background(),
		`SELECT subject_type, subject_id, provider_instance_id, capabilities, updated_at
		 FROM canonical_capability_sets
		 WHERE subject_type = $1 AND subject_id = $2`,
		strings.ToLower(strings.TrimSpace(subjectType)),
		strings.TrimSpace(subjectID),
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.CapabilitySet{}, false, nil
		}
		return model.CapabilitySet{}, false, err
	}
	return set, true, nil
}

func (s *PostgresStore) ReplaceCapabilitySets(providerInstanceID string, sets []model.CapabilitySet) error {
	providerInstanceID = strings.TrimSpace(providerInstanceID)
	if providerInstanceID == "" {
		return ErrNotFound
	}

	tx, err := s.pool.Begin(context.Background())
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())

	if _, err := tx.Exec(
		context.Background(),
		`DELETE FROM canonical_capability_sets
		 WHERE provider_instance_id = $1
		    OR (subject_type = 'provider' AND subject_id = $1)`,
		providerInstanceID,
	); err != nil {
		return err
	}

	now := time.Now().UTC()
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
		payload, payloadErr := marshalCapabilitySpecs(set.Capabilities)
		if payloadErr != nil {
			return payloadErr
		}
		updatedAt := set.UpdatedAt.UTC()
		if updatedAt.IsZero() {
			updatedAt = now
		}
		if _, err := tx.Exec(
			context.Background(),
			`INSERT INTO canonical_capability_sets (
				subject_type, subject_id, provider_instance_id, capabilities, updated_at
			)
			VALUES ($1, $2, $3, $4::jsonb, $5)
			ON CONFLICT (subject_type, subject_id) DO UPDATE SET
				provider_instance_id = EXCLUDED.provider_instance_id,
				capabilities = EXCLUDED.capabilities,
				updated_at = EXCLUDED.updated_at`,
			subjectType,
			subjectID,
			providerInstanceID,
			payload,
			updatedAt,
		); err != nil {
			return err
		}
	}

	return tx.Commit(context.Background())
}

func (s *PostgresStore) ListCapabilitySets(limit int) ([]model.CapabilitySet, error) {
	if limit <= 0 {
		limit = 500
	}
	if limit > 5000 {
		limit = 5000
	}

	rows, err := s.pool.Query(
		context.Background(),
		`SELECT subject_type, subject_id, provider_instance_id, capabilities, updated_at
		 FROM canonical_capability_sets
		 ORDER BY updated_at DESC
		 LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]model.CapabilitySet, 0, limit)
	for rows.Next() {
		set, _, scanErr := scanCapabilitySet(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, set)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (s *PostgresStore) UpsertTemplateBinding(binding model.TemplateBinding) (model.TemplateBinding, error) {
	resourceID := strings.TrimSpace(binding.ResourceID)
	if resourceID == "" {
		return model.TemplateBinding{}, ErrNotFound
	}

	tabsPayload, err := marshalStringSlice(binding.Tabs)
	if err != nil {
		return model.TemplateBinding{}, err
	}
	operationsPayload, err := marshalStringSlice(binding.Operations)
	if err != nil {
		return model.TemplateBinding{}, err
	}
	updatedAt := binding.UpdatedAt.UTC()
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}

	stored, err := scanTemplateBinding(s.pool.QueryRow(
		context.Background(),
		`INSERT INTO canonical_template_bindings (
			resource_id, template_id, tabs, operations, updated_at
		)
		VALUES ($1, $2, $3::jsonb, $4::jsonb, $5)
		ON CONFLICT (resource_id) DO UPDATE SET
			template_id = EXCLUDED.template_id,
			tabs = EXCLUDED.tabs,
			operations = EXCLUDED.operations,
			updated_at = EXCLUDED.updated_at
		RETURNING resource_id, template_id, tabs, operations, updated_at`,
		resourceID,
		firstNonEmpty(strings.TrimSpace(binding.TemplateID), "template.other.default"),
		tabsPayload,
		operationsPayload,
		updatedAt,
	))
	if err != nil {
		return model.TemplateBinding{}, err
	}
	return stored, nil
}

func (s *PostgresStore) GetTemplateBinding(resourceID string) (model.TemplateBinding, bool, error) {
	binding, err := scanTemplateBinding(s.pool.QueryRow(
		context.Background(),
		`SELECT resource_id, template_id, tabs, operations, updated_at
		 FROM canonical_template_bindings
		 WHERE resource_id = $1`,
		strings.TrimSpace(resourceID),
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.TemplateBinding{}, false, nil
		}
		return model.TemplateBinding{}, false, err
	}
	return binding, true, nil
}

func (s *PostgresStore) ListTemplateBindings(resourceIDs []string) ([]model.TemplateBinding, error) {
	var (
		rows pgx.Rows
		err  error
	)
	if len(resourceIDs) == 0 {
		rows, err = s.pool.Query(
			context.Background(),
			`SELECT resource_id, template_id, tabs, operations, updated_at
			 FROM canonical_template_bindings
			 ORDER BY updated_at DESC`,
		)
	} else {
		cleaned := make([]string, 0, len(resourceIDs))
		for _, resourceID := range resourceIDs {
			resourceID = strings.TrimSpace(resourceID)
			if resourceID == "" {
				continue
			}
			cleaned = append(cleaned, resourceID)
		}
		if len(cleaned) == 0 {
			return nil, nil
		}
		rows, err = s.pool.Query(
			context.Background(),
			`SELECT resource_id, template_id, tabs, operations, updated_at
			 FROM canonical_template_bindings
			 WHERE resource_id = ANY($1)
			 ORDER BY updated_at DESC`,
			cleaned,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]model.TemplateBinding, 0, 64)
	for rows.Next() {
		binding, scanErr := scanTemplateBinding(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, binding)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (s *PostgresStore) UpsertIngestCheckpoint(checkpoint model.IngestCheckpoint) (model.IngestCheckpoint, error) {
	providerID := strings.TrimSpace(checkpoint.ProviderInstanceID)
	stream := strings.ToLower(strings.TrimSpace(checkpoint.Stream))
	if providerID == "" || stream == "" {
		return model.IngestCheckpoint{}, ErrNotFound
	}
	syncedAt := checkpoint.SyncedAt.UTC()
	if syncedAt.IsZero() {
		syncedAt = time.Now().UTC()
	}

	stored, err := scanIngestCheckpoint(s.pool.QueryRow(
		context.Background(),
		`INSERT INTO canonical_ingest_checkpoints (
			provider_instance_id, stream, cursor, synced_at
		)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (provider_instance_id, stream) DO UPDATE SET
			cursor = EXCLUDED.cursor,
			synced_at = EXCLUDED.synced_at
		RETURNING provider_instance_id, stream, cursor, synced_at`,
		providerID,
		stream,
		nullIfBlank(checkpoint.Cursor),
		syncedAt,
	))
	if err != nil {
		return model.IngestCheckpoint{}, err
	}
	return stored, nil
}

func (s *PostgresStore) GetIngestCheckpoint(providerInstanceID, stream string) (model.IngestCheckpoint, bool, error) {
	checkpoint, err := scanIngestCheckpoint(s.pool.QueryRow(
		context.Background(),
		`SELECT provider_instance_id, stream, cursor, synced_at
		 FROM canonical_ingest_checkpoints
		 WHERE provider_instance_id = $1 AND stream = $2`,
		strings.TrimSpace(providerInstanceID),
		strings.ToLower(strings.TrimSpace(stream)),
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.IngestCheckpoint{}, false, nil
		}
		return model.IngestCheckpoint{}, false, err
	}
	return checkpoint, true, nil
}

func (s *PostgresStore) RecordReconciliationResult(result model.ReconciliationResult) (model.ReconciliationResult, error) {
	providerID := strings.TrimSpace(result.ProviderInstanceID)
	if providerID == "" {
		return model.ReconciliationResult{}, ErrNotFound
	}
	startedAt := result.StartedAt.UTC()
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	finishedAt := result.FinishedAt.UTC()
	if finishedAt.IsZero() {
		finishedAt = time.Now().UTC()
	}

	stored, _, err := scanReconciliationResult(s.pool.QueryRow(
		context.Background(),
		`INSERT INTO canonical_reconciliation_results (
			id, provider_instance_id, created_count, updated_count,
			stale_count, error_count, started_at, finished_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, provider_instance_id, created_count, updated_count,
			stale_count, error_count, started_at, finished_at`,
		idgen.New("reconcile"),
		providerID,
		result.CreatedCount,
		result.UpdatedCount,
		result.StaleCount,
		result.ErrorCount,
		startedAt,
		finishedAt,
	))
	if err != nil {
		return model.ReconciliationResult{}, err
	}
	return stored, nil
}

func (s *PostgresStore) ListReconciliationResults(providerInstanceID string, limit int) ([]model.ReconciliationResult, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 5000 {
		limit = 5000
	}
	providerInstanceID = strings.TrimSpace(providerInstanceID)

	var (
		rows pgx.Rows
		err  error
	)
	if providerInstanceID == "" {
		rows, err = s.pool.Query(
			context.Background(),
			`SELECT id, provider_instance_id, created_count, updated_count,
				stale_count, error_count, started_at, finished_at
			 FROM canonical_reconciliation_results
			 ORDER BY finished_at DESC
			 LIMIT $1`,
			limit,
		)
	} else {
		rows, err = s.pool.Query(
			context.Background(),
			`SELECT id, provider_instance_id, created_count, updated_count,
				stale_count, error_count, started_at, finished_at
			 FROM canonical_reconciliation_results
			 WHERE provider_instance_id = $1
			 ORDER BY finished_at DESC
			 LIMIT $2`,
			providerInstanceID,
			limit,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]model.ReconciliationResult, 0, limit)
	for rows.Next() {
		result, _, scanErr := scanReconciliationResult(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, result)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (s *PostgresStore) CanonicalStatusWatermark() (time.Time, error) {
	var watermark time.Time
	if err := s.pool.QueryRow(
		context.Background(),
		`SELECT GREATEST(
			COALESCE((SELECT MAX(updated_at) FROM provider_instances), to_timestamp(0)),
			COALESCE((SELECT MAX(updated_at) FROM canonical_capability_sets), to_timestamp(0)),
			COALESCE((SELECT MAX(updated_at) FROM canonical_template_bindings), to_timestamp(0)),
			COALESCE((SELECT MAX(finished_at) FROM canonical_reconciliation_results), to_timestamp(0)),
			COALESCE((SELECT MAX(updated_at) FROM resource_external_refs), to_timestamp(0)),
			COALESCE((SELECT MAX(updated_at) FROM canonical_resource_relationships), to_timestamp(0)),
			COALESCE((SELECT MAX(synced_at) FROM canonical_ingest_checkpoints), to_timestamp(0))
		)`,
	).Scan(&watermark); err != nil {
		return time.Time{}, err
	}
	return watermark.UTC(), nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

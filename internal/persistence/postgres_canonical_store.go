package persistence

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/labtether/labtether/internal/model"
	"strings"
	"time"
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

	out := make([]model.ProviderInstance, 0)
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

	out := make([]model.ResourceRelationship, 0)
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

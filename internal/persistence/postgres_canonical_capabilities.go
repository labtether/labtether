package persistence

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/labtether/labtether/internal/model"
	"strings"
	"time"
)

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

	out := make([]model.CapabilitySet, 0)
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

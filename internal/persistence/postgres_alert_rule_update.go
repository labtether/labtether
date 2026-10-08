package persistence

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/idgen"
	"strings"
	"time"
)

func (s *PostgresStore) UpdateAlertRule(id string, req alerts.UpdateRuleRequest) (alerts.Rule, error) {
	id = strings.TrimSpace(id)
	ctx := context.Background()

	// Validate request fields before acquiring the transaction so we fail fast
	// without holding a DB connection for pure input validation.
	if req.Status != nil {
		if alerts.NormalizeRuleStatus(*req.Status) == "" {
			return alerts.Rule{}, errors.New("invalid alert rule status")
		}
	}
	if req.Severity != nil {
		if alerts.NormalizeSeverity(*req.Severity) == "" {
			return alerts.Rule{}, errors.New("invalid alert rule severity")
		}
	}
	if req.CooldownSeconds != nil {
		if err := validateStoredAlertDurationSeconds("cooldown_seconds", *req.CooldownSeconds); err != nil {
			return alerts.Rule{}, err
		}
	}
	if req.ReopenAfterSeconds != nil {
		if err := validateStoredAlertDurationSeconds("reopen_after_seconds", *req.ReopenAfterSeconds); err != nil {
			return alerts.Rule{}, err
		}
	}
	if req.EvaluationIntervalSeconds != nil {
		if err := validateStoredAlertDurationSeconds("evaluation_interval_seconds", *req.EvaluationIntervalSeconds); err != nil {
			return alerts.Rule{}, err
		}
	}
	if req.WindowSeconds != nil {
		if err := validateStoredAlertDurationSeconds("window_seconds", *req.WindowSeconds); err != nil {
			return alerts.Rule{}, err
		}
	}

	// Wrap the read-then-write in a single transaction to prevent TOCTOU races.
	// SELECT FOR UPDATE locks the row so no concurrent UPDATE can modify it between
	// our read and our write.
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return alerts.Rule{}, err
	}
	defer tx.Rollback(ctx)

	rule, err := scanAlertRule(tx.QueryRow(ctx,
		`SELECT
			id,
			name,
			description,
			status,
			kind,
			severity,
			target_scope,
			cooldown_seconds,
			reopen_after_seconds,
			evaluation_interval_seconds,
			window_seconds,
			condition,
			labels,
			metadata,
			created_by,
			created_at,
			updated_at,
			last_evaluated_at
		 FROM alert_rules
		 WHERE id = $1
		 FOR UPDATE`,
		id,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return alerts.Rule{}, alerts.ErrRuleNotFound
		}
		return alerts.Rule{}, err
	}

	if req.Name != nil {
		rule.Name = strings.TrimSpace(*req.Name)
	}
	if req.Description != nil {
		rule.Description = strings.TrimSpace(*req.Description)
	}
	if req.Status != nil {
		rule.Status = alerts.NormalizeRuleStatus(*req.Status)
	}
	if req.Severity != nil {
		rule.Severity = alerts.NormalizeSeverity(*req.Severity)
	}
	if req.CooldownSeconds != nil {
		rule.CooldownSeconds = *req.CooldownSeconds
	}
	if req.ReopenAfterSeconds != nil {
		rule.ReopenAfterSeconds = *req.ReopenAfterSeconds
	}
	if req.EvaluationIntervalSeconds != nil {
		interval := *req.EvaluationIntervalSeconds
		if interval <= 0 {
			interval = alerts.DefaultEvaluationIntervalSeconds
		}
		rule.EvaluationIntervalSeconds = interval
	}
	if req.WindowSeconds != nil {
		window := *req.WindowSeconds
		if window <= 0 {
			window = alerts.DefaultWindowSeconds
		}
		rule.WindowSeconds = window
	}
	if req.Condition != nil {
		rule.Condition = cloneAnyMap(*req.Condition)
	}
	if req.Labels != nil {
		rule.Labels = cloneMetadata(*req.Labels)
	}
	if req.Metadata != nil {
		rule.Metadata = cloneMetadata(*req.Metadata)
	}

	conditionPayload, err := marshalAnyMap(rule.Condition)
	if err != nil {
		return alerts.Rule{}, err
	}
	labelsPayload, err := marshalStringMap(rule.Labels)
	if err != nil {
		return alerts.Rule{}, err
	}
	metadataPayload, err := marshalStringMap(rule.Metadata)
	if err != nil {
		return alerts.Rule{}, err
	}

	now := time.Now().UTC()
	tag, err := tx.Exec(ctx,
		`UPDATE alert_rules
		 SET name = $2,
		     description = $3,
		     status = $4,
		     severity = $5,
		     cooldown_seconds = $6,
		     reopen_after_seconds = $7,
		     evaluation_interval_seconds = $8,
		     window_seconds = $9,
		     condition = $10::jsonb,
		     labels = $11::jsonb,
		     metadata = $12::jsonb,
		     updated_at = $13
		 WHERE id = $1`,
		rule.ID,
		rule.Name,
		rule.Description,
		rule.Status,
		rule.Severity,
		rule.CooldownSeconds,
		rule.ReopenAfterSeconds,
		rule.EvaluationIntervalSeconds,
		rule.WindowSeconds,
		conditionPayload,
		labelsPayload,
		metadataPayload,
		now,
	)
	if err != nil {
		return alerts.Rule{}, err
	}
	if tag.RowsAffected() == 0 {
		return alerts.Rule{}, alerts.ErrRuleNotFound
	}

	if req.Targets != nil {
		if _, err := tx.Exec(ctx,
			`DELETE FROM alert_rule_targets WHERE rule_id = $1`,
			rule.ID,
		); err != nil {
			return alerts.Rule{}, err
		}
		for _, target := range *req.Targets {
			assetID := strings.TrimSpace(target.AssetID)
			groupID := strings.TrimSpace(target.GroupID)
			selectorArg := any(nil)
			if len(target.Selector) > 0 {
				selectorPayload, selectorErr := marshalAnyMap(target.Selector)
				if selectorErr != nil {
					return alerts.Rule{}, selectorErr
				}
				selectorArg = selectorPayload
			}
			if _, err := tx.Exec(ctx,
				`INSERT INTO alert_rule_targets (id, rule_id, asset_id, group_id, selector, created_at)
				 VALUES ($1, $2, $3, $4, $5::jsonb, $6)`,
				idgen.New("art"),
				rule.ID,
				nullIfBlank(assetID),
				nullIfBlank(groupID),
				selectorArg,
				now,
			); err != nil {
				return alerts.Rule{}, err
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return alerts.Rule{}, err
	}

	// Re-fetch after commit to return consistent, fully-populated state.
	// GetAlertRule reads outside the committed transaction, which is correct here.
	updated, ok, err := s.GetAlertRule(rule.ID)
	if err != nil {
		return alerts.Rule{}, err
	}
	if !ok {
		return alerts.Rule{}, alerts.ErrRuleNotFound
	}
	return updated, nil
}

func validateStoredAlertDurationSeconds(field string, value int) error {
	if value < 0 {
		return fmt.Errorf("%s must be >= 0", field)
	}
	if value > alerts.MaxDurationSeconds {
		return fmt.Errorf("%s is out of range", field)
	}
	return nil
}

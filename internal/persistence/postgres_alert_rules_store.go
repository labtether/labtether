package persistence

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/idgen"
)

func (s *PostgresStore) CreateAlertRule(req alerts.CreateRuleRequest) (alerts.Rule, error) {
	now := time.Now().UTC()

	status := alerts.NormalizeRuleStatus(req.Status)
	if status == "" {
		status = alerts.RuleStatusActive
	}
	kind := alerts.NormalizeRuleKind(req.Kind)
	if kind == "" {
		kind = alerts.RuleKindMetricThreshold
	}
	severity := alerts.NormalizeSeverity(req.Severity)
	if severity == "" {
		severity = alerts.SeverityMedium
	}
	targetScope := alerts.NormalizeTargetScope(req.TargetScope)
	if targetScope == "" {
		targetScope = alerts.TargetScopeGlobal
	}

	cooldownSeconds := req.CooldownSeconds
	if cooldownSeconds < 0 {
		cooldownSeconds = 0
	}
	if cooldownSeconds == 0 {
		cooldownSeconds = 300
	}
	reopenAfterSeconds := req.ReopenAfterSeconds
	if reopenAfterSeconds < 0 {
		reopenAfterSeconds = 0
	}
	if reopenAfterSeconds == 0 {
		reopenAfterSeconds = 60
	}
	evaluationIntervalSeconds := req.EvaluationIntervalSeconds
	if evaluationIntervalSeconds <= 0 {
		evaluationIntervalSeconds = 30
	}
	windowSeconds := req.WindowSeconds
	if windowSeconds <= 0 {
		windowSeconds = 300
	}

	conditionPayload, err := marshalAnyMap(req.Condition)
	if err != nil {
		return alerts.Rule{}, err
	}
	labelsPayload, err := marshalStringMap(req.Labels)
	if err != nil {
		return alerts.Rule{}, err
	}
	metadataPayload, err := marshalStringMap(req.Metadata)
	if err != nil {
		return alerts.Rule{}, err
	}

	createdBy := strings.TrimSpace(req.CreatedBy)
	if createdBy == "" {
		createdBy = "owner"
	}

	ruleID := idgen.New("arl")
	tx, err := s.pool.Begin(context.Background())
	if err != nil {
		return alerts.Rule{}, err
	}
	defer tx.Rollback(context.Background())

	if _, err := tx.Exec(context.Background(),
		`INSERT INTO alert_rules (
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
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12::jsonb, $13::jsonb, $14::jsonb, $15, $16, $16, NULL)`,
		ruleID,
		strings.TrimSpace(req.Name),
		strings.TrimSpace(req.Description),
		status,
		kind,
		severity,
		targetScope,
		cooldownSeconds,
		reopenAfterSeconds,
		evaluationIntervalSeconds,
		windowSeconds,
		conditionPayload,
		labelsPayload,
		metadataPayload,
		createdBy,
		now,
	); err != nil {
		return alerts.Rule{}, err
	}

	for _, target := range req.Targets {
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

		if _, err := tx.Exec(context.Background(),
			`INSERT INTO alert_rule_targets (id, rule_id, asset_id, group_id, selector, created_at)
			 VALUES ($1, $2, $3, $4, $5::jsonb, $6)`,
			idgen.New("art"),
			ruleID,
			nullIfBlank(assetID),
			nullIfBlank(groupID),
			selectorArg,
			now,
		); err != nil {
			return alerts.Rule{}, err
		}
	}

	if err := tx.Commit(context.Background()); err != nil {
		return alerts.Rule{}, err
	}

	created, ok, err := s.GetAlertRule(ruleID)
	if err != nil {
		return alerts.Rule{}, err
	}
	if !ok {
		return alerts.Rule{}, alerts.ErrRuleNotFound
	}
	return created, nil
}

func (s *PostgresStore) GetAlertRule(id string) (alerts.Rule, bool, error) {
	rule, err := scanAlertRule(s.pool.QueryRow(context.Background(),
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
		 WHERE id = $1`,
		strings.TrimSpace(id),
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return alerts.Rule{}, false, nil
		}
		return alerts.Rule{}, false, err
	}

	targets, err := s.listAlertRuleTargets(rule.ID)
	if err != nil {
		return alerts.Rule{}, false, err
	}
	rule.Targets = targets
	return rule, true, nil
}

func (s *PostgresStore) ListAlertRules(filter AlertRuleFilter) ([]alerts.Rule, error) {
	ctx := context.Background()

	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	where := make([]string, 0, 3)
	args := make([]any, 0, 4)
	next := 1

	if status := alerts.NormalizeRuleStatus(filter.Status); status != "" {
		where = append(where, fmt.Sprintf("status = $%d", next))
		args = append(args, status)
		next++
	}
	if kind := alerts.NormalizeRuleKind(filter.Kind); kind != "" {
		where = append(where, fmt.Sprintf("kind = $%d", next))
		args = append(args, kind)
		next++
	}
	if severity := alerts.NormalizeSeverity(filter.Severity); severity != "" {
		where = append(where, fmt.Sprintf("severity = $%d", next))
		args = append(args, severity)
		next++
	}

	sql := `SELECT
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
	FROM alert_rules`
	if len(where) > 0 {
		sql += " WHERE " + strings.Join(where, " AND ")
	}
	sql += fmt.Sprintf(" ORDER BY updated_at DESC LIMIT $%d", next)
	args = append(args, limit)
	next++
	if filter.Offset > 0 {
		args = append(args, filter.Offset)
		sql += fmt.Sprintf(" OFFSET $%d", next)
	}

	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]alerts.Rule, 0, limit)
	ruleIDs := make([]string, 0, limit)
	for rows.Next() {
		rule, scanErr := scanAlertRule(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, rule)
		ruleIDs = append(ruleIDs, rule.ID)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	rows.Close()

	// Single batch query for all targets — eliminates N+1 (one query per rule).
	targetsByRule, err := s.listAlertRuleTargetsBatch(ctx, ruleIDs)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Targets = targetsByRule[out[i].ID]
		if out[i].Targets == nil {
			out[i].Targets = []alerts.RuleTarget{}
		}
	}

	return out, nil
}

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
	if req.CooldownSeconds != nil && *req.CooldownSeconds < 0 {
		return alerts.Rule{}, errors.New("cooldown_seconds must be >= 0")
	}
	if req.ReopenAfterSeconds != nil && *req.ReopenAfterSeconds < 0 {
		return alerts.Rule{}, errors.New("reopen_after_seconds must be >= 0")
	}
	if req.EvaluationIntervalSeconds != nil && *req.EvaluationIntervalSeconds <= 0 {
		return alerts.Rule{}, errors.New("evaluation_interval_seconds must be > 0")
	}
	if req.WindowSeconds != nil && *req.WindowSeconds <= 0 {
		return alerts.Rule{}, errors.New("window_seconds must be > 0")
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
		rule.EvaluationIntervalSeconds = *req.EvaluationIntervalSeconds
	}
	if req.WindowSeconds != nil {
		rule.WindowSeconds = *req.WindowSeconds
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

func (s *PostgresStore) DeleteAlertRule(id string) error {
	tag, err := s.pool.Exec(context.Background(),
		`DELETE FROM alert_rules WHERE id = $1`,
		strings.TrimSpace(id),
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return alerts.ErrRuleNotFound
	}
	return nil
}

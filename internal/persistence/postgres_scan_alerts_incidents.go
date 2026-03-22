package persistence

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/incidents"
)

type alertRuleScanner interface {
	Scan(dest ...any) error
}

func scanAlertRule(row alertRuleScanner) (alerts.Rule, error) {
	rule := alerts.Rule{}
	var condition []byte
	var labels []byte
	var metadata []byte
	var lastEvaluatedAt *time.Time
	if err := row.Scan(
		&rule.ID,
		&rule.Name,
		&rule.Description,
		&rule.Status,
		&rule.Kind,
		&rule.Severity,
		&rule.TargetScope,
		&rule.CooldownSeconds,
		&rule.ReopenAfterSeconds,
		&rule.EvaluationIntervalSeconds,
		&rule.WindowSeconds,
		&condition,
		&labels,
		&metadata,
		&rule.CreatedBy,
		&rule.CreatedAt,
		&rule.UpdatedAt,
		&lastEvaluatedAt,
	); err != nil {
		return alerts.Rule{}, err
	}

	rule.Condition = unmarshalAnyMap(condition)
	rule.Labels = unmarshalStringMap(labels)
	rule.Metadata = unmarshalStringMap(metadata)
	if lastEvaluatedAt != nil {
		value := lastEvaluatedAt.UTC()
		rule.LastEvaluatedAt = &value
	}
	return rule, nil
}

// listAlertRuleTargets fetches targets for a single rule ID. Used by GetAlertRule.
func (s *PostgresStore) listAlertRuleTargets(ruleID string) ([]alerts.RuleTarget, error) {
	return s.listAlertRuleTargetsCtx(context.Background(), ruleID)
}

// listAlertRuleTargetsCtx fetches targets for a single rule ID using the provided context.
func (s *PostgresStore) listAlertRuleTargetsCtx(ctx context.Context, ruleID string) ([]alerts.RuleTarget, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, rule_id, asset_id, group_id, selector, created_at
		 FROM alert_rule_targets
		 WHERE rule_id = $1
		 ORDER BY created_at ASC`,
		ruleID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]alerts.RuleTarget, 0, 8)
	for rows.Next() {
		target := alerts.RuleTarget{}
		var assetID *string
		var groupID *string
		var selector []byte
		if err := rows.Scan(
			&target.ID,
			&target.RuleID,
			&assetID,
			&groupID,
			&selector,
			&target.CreatedAt,
		); err != nil {
			return nil, err
		}
		if assetID != nil {
			target.AssetID = *assetID
		}
		if groupID != nil {
			target.GroupID = *groupID
		}
		target.Selector = unmarshalAnyMap(selector)
		target.CreatedAt = target.CreatedAt.UTC()
		out = append(out, target)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

// listAlertRuleTargetsBatch fetches targets for multiple rule IDs in a single query
// and returns a map from rule ID to its targets. Used by ListAlertRules to avoid N+1.
func (s *PostgresStore) listAlertRuleTargetsBatch(ctx context.Context, ruleIDs []string) (map[string][]alerts.RuleTarget, error) {
	result := make(map[string][]alerts.RuleTarget, len(ruleIDs))
	if len(ruleIDs) == 0 {
		return result, nil
	}

	rows, err := s.pool.Query(ctx,
		`SELECT id, rule_id, asset_id, group_id, selector, created_at
		 FROM alert_rule_targets
		 WHERE rule_id = ANY($1::text[])
		 ORDER BY rule_id, created_at ASC`,
		ruleIDs,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		target := alerts.RuleTarget{}
		var assetID *string
		var groupID *string
		var selector []byte
		if err := rows.Scan(
			&target.ID,
			&target.RuleID,
			&assetID,
			&groupID,
			&selector,
			&target.CreatedAt,
		); err != nil {
			return nil, err
		}
		if assetID != nil {
			target.AssetID = *assetID
		}
		if groupID != nil {
			target.GroupID = *groupID
		}
		target.Selector = unmarshalAnyMap(selector)
		target.CreatedAt = target.CreatedAt.UTC()
		result[target.RuleID] = append(result[target.RuleID], target)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return result, nil
}

type alertEvaluationScanner interface {
	Scan(dest ...any) error
}

func scanAlertEvaluation(row alertEvaluationScanner) (alerts.Evaluation, error) {
	evaluation := alerts.Evaluation{}
	var details []byte
	if err := row.Scan(
		&evaluation.ID,
		&evaluation.RuleID,
		&evaluation.Status,
		&evaluation.EvaluatedAt,
		&evaluation.DurationMS,
		&evaluation.CandidateCount,
		&evaluation.TriggeredCount,
		&evaluation.Error,
		&details,
	); err != nil {
		return alerts.Evaluation{}, err
	}
	evaluation.Details = unmarshalAnyMap(details)
	evaluation.EvaluatedAt = evaluation.EvaluatedAt.UTC()
	return evaluation, nil
}

type incidentScanner interface {
	Scan(dest ...any) error
}

func scanIncident(row incidentScanner) (incidents.Incident, error) {
	incident := incidents.Incident{}
	var groupID *string
	var primaryAssetID *string
	var assignee *string
	var mitigatedAt *time.Time
	var resolvedAt *time.Time
	var closedAt *time.Time
	var metadata []byte
	var actionItemsJSON []byte
	if err := row.Scan(
		&incident.ID,
		&incident.Title,
		&incident.Summary,
		&incident.Status,
		&incident.Severity,
		&incident.Source,
		&groupID,
		&primaryAssetID,
		&assignee,
		&incident.CreatedBy,
		&incident.OpenedAt,
		&mitigatedAt,
		&resolvedAt,
		&closedAt,
		&metadata,
		&incident.RootCause,
		&actionItemsJSON,
		&incident.LessonsLearned,
		&incident.CreatedAt,
		&incident.UpdatedAt,
	); err != nil {
		return incidents.Incident{}, err
	}
	if groupID != nil {
		incident.GroupID = *groupID
	}
	if primaryAssetID != nil {
		incident.PrimaryAssetID = *primaryAssetID
	}
	if assignee != nil {
		incident.Assignee = *assignee
	}
	if mitigatedAt != nil {
		value := mitigatedAt.UTC()
		incident.MitigatedAt = &value
	}
	if resolvedAt != nil {
		value := resolvedAt.UTC()
		incident.ResolvedAt = &value
	}
	if closedAt != nil {
		value := closedAt.UTC()
		incident.ClosedAt = &value
	}

	incident.Metadata = unmarshalStringMap(metadata)
	if len(actionItemsJSON) > 0 {
		if err := json.Unmarshal(actionItemsJSON, &incident.ActionItems); err != nil {
			return incidents.Incident{}, fmt.Errorf("corrupt action_items JSON for incident %s: %w", incident.ID, err)
		}
	}
	incident.OpenedAt = incident.OpenedAt.UTC()
	incident.CreatedAt = incident.CreatedAt.UTC()
	incident.UpdatedAt = incident.UpdatedAt.UTC()
	return incident, nil
}

type incidentAlertLinkScanner interface {
	Scan(dest ...any) error
}

func scanIncidentAlertLink(row incidentAlertLinkScanner) (incidents.AlertLink, error) {
	link := incidents.AlertLink{}
	var alertRuleID *string
	var alertInstanceID *string
	var alertFingerprint *string
	if err := row.Scan(
		&link.ID,
		&link.IncidentID,
		&alertRuleID,
		&alertInstanceID,
		&alertFingerprint,
		&link.LinkType,
		&link.CreatedBy,
		&link.CreatedAt,
	); err != nil {
		return incidents.AlertLink{}, err
	}
	if alertRuleID != nil {
		link.AlertRuleID = *alertRuleID
	}
	if alertInstanceID != nil {
		link.AlertInstanceID = *alertInstanceID
	}
	if alertFingerprint != nil {
		link.AlertFingerprint = *alertFingerprint
	}
	link.CreatedAt = link.CreatedAt.UTC()
	return link, nil
}

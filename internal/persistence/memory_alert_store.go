package persistence

import (
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/idgen"
)

type MemoryAlertStore struct {
	mu          sync.RWMutex
	rules       map[string]alerts.Rule
	evaluations map[string][]alerts.Evaluation
}

func NewMemoryAlertStore() *MemoryAlertStore {
	return &MemoryAlertStore{
		rules:       make(map[string]alerts.Rule),
		evaluations: make(map[string][]alerts.Evaluation),
	}
}

func (m *MemoryAlertStore) CreateAlertRule(req alerts.CreateRuleRequest) (alerts.Rule, error) {
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
	scope := alerts.NormalizeTargetScope(req.TargetScope)
	if scope == "" {
		scope = alerts.TargetScopeGlobal
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

	createdBy := strings.TrimSpace(req.CreatedBy)
	if createdBy == "" {
		createdBy = "owner"
	}

	rule := alerts.Rule{
		ID:                        idgen.New("arl"),
		Name:                      strings.TrimSpace(req.Name),
		Description:               strings.TrimSpace(req.Description),
		Status:                    status,
		Kind:                      kind,
		Severity:                  severity,
		TargetScope:               scope,
		CooldownSeconds:           cooldownSeconds,
		ReopenAfterSeconds:        reopenAfterSeconds,
		EvaluationIntervalSeconds: evaluationIntervalSeconds,
		WindowSeconds:             windowSeconds,
		Condition:                 cloneAnyMap(req.Condition),
		Labels:                    cloneMetadata(req.Labels),
		Metadata:                  cloneMetadata(req.Metadata),
		CreatedBy:                 createdBy,
		CreatedAt:                 now,
		UpdatedAt:                 now,
	}
	if len(req.Targets) > 0 {
		rule.Targets = make([]alerts.RuleTarget, 0, len(req.Targets))
		for _, target := range req.Targets {
			rule.Targets = append(rule.Targets, alerts.RuleTarget{
				ID:        idgen.New("art"),
				RuleID:    rule.ID,
				AssetID:   strings.TrimSpace(target.AssetID),
				GroupID:   strings.TrimSpace(target.GroupID),
				Selector:  cloneAnyMap(target.Selector),
				CreatedAt: now,
			})
		}
	}

	m.mu.Lock()
	m.rules[rule.ID] = cloneAlertRule(rule)
	m.mu.Unlock()
	return cloneAlertRule(rule), nil
}

func (m *MemoryAlertStore) GetAlertRule(id string) (alerts.Rule, bool, error) {
	m.mu.RLock()
	rule, ok := m.rules[id]
	m.mu.RUnlock()
	if !ok {
		return alerts.Rule{}, false, nil
	}
	return cloneAlertRule(rule), true, nil
}

func (m *MemoryAlertStore) ListAlertRules(filter AlertRuleFilter) ([]alerts.Rule, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	status := alerts.NormalizeRuleStatus(filter.Status)
	kind := alerts.NormalizeRuleKind(filter.Kind)
	severity := alerts.NormalizeSeverity(filter.Severity)

	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]alerts.Rule, 0, len(m.rules))
	for _, rule := range m.rules {
		if status != "" && rule.Status != status {
			continue
		}
		if kind != "" && rule.Kind != kind {
			continue
		}
		if severity != "" && rule.Severity != severity {
			continue
		}
		out = append(out, cloneAlertRule(rule))
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	if filter.Offset > 0 {
		if filter.Offset >= len(out) {
			return []alerts.Rule{}, nil
		}
		out = out[filter.Offset:]
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *MemoryAlertStore) UpdateAlertRule(id string, req alerts.UpdateRuleRequest) (alerts.Rule, error) {
	now := time.Now().UTC()

	m.mu.Lock()
	defer m.mu.Unlock()

	rule, ok := m.rules[id]
	if !ok {
		return alerts.Rule{}, alerts.ErrRuleNotFound
	}

	if req.Name != nil {
		rule.Name = strings.TrimSpace(*req.Name)
	}
	if req.Description != nil {
		rule.Description = strings.TrimSpace(*req.Description)
	}
	if req.Status != nil {
		if status := alerts.NormalizeRuleStatus(*req.Status); status != "" {
			rule.Status = status
		}
	}
	if req.Severity != nil {
		if severity := alerts.NormalizeSeverity(*req.Severity); severity != "" {
			rule.Severity = severity
		}
	}
	if req.CooldownSeconds != nil {
		cooldown := *req.CooldownSeconds
		if cooldown < 0 {
			cooldown = 0
		}
		rule.CooldownSeconds = cooldown
	}
	if req.ReopenAfterSeconds != nil {
		reopenAfter := *req.ReopenAfterSeconds
		if reopenAfter < 0 {
			reopenAfter = 0
		}
		rule.ReopenAfterSeconds = reopenAfter
	}
	if req.EvaluationIntervalSeconds != nil {
		interval := *req.EvaluationIntervalSeconds
		if interval <= 0 {
			interval = 30
		}
		rule.EvaluationIntervalSeconds = interval
	}
	if req.WindowSeconds != nil {
		window := *req.WindowSeconds
		if window <= 0 {
			window = 300
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
	if req.Targets != nil {
		targets := make([]alerts.RuleTarget, 0, len(*req.Targets))
		for _, target := range *req.Targets {
			targets = append(targets, alerts.RuleTarget{
				ID:        idgen.New("art"),
				RuleID:    rule.ID,
				AssetID:   strings.TrimSpace(target.AssetID),
				GroupID:   strings.TrimSpace(target.GroupID),
				Selector:  cloneAnyMap(target.Selector),
				CreatedAt: now,
			})
		}
		rule.Targets = targets
	}

	rule.UpdatedAt = now
	m.rules[id] = cloneAlertRule(rule)
	return cloneAlertRule(rule), nil
}

func (m *MemoryAlertStore) DeleteAlertRule(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.rules[id]; !ok {
		return alerts.ErrRuleNotFound
	}
	delete(m.rules, id)
	delete(m.evaluations, id)
	return nil
}

func (m *MemoryAlertStore) RecordAlertEvaluation(ruleID string, evaluation alerts.Evaluation) (alerts.Evaluation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	rule, ok := m.rules[strings.TrimSpace(ruleID)]
	if !ok {
		return alerts.Evaluation{}, alerts.ErrRuleNotFound
	}

	evaluatedAt := evaluation.EvaluatedAt.UTC()
	if evaluatedAt.IsZero() {
		evaluatedAt = time.Now().UTC()
	}

	status := alerts.NormalizeEvaluationStatus(evaluation.Status)
	if status == "" {
		status = alerts.EvaluationStatusError
	}

	created := alerts.Evaluation{
		ID:             strings.TrimSpace(evaluation.ID),
		RuleID:         strings.TrimSpace(ruleID),
		Status:         status,
		EvaluatedAt:    evaluatedAt,
		DurationMS:     evaluation.DurationMS,
		CandidateCount: evaluation.CandidateCount,
		TriggeredCount: evaluation.TriggeredCount,
		Error:          strings.TrimSpace(evaluation.Error),
		Details:        cloneAnyMap(evaluation.Details),
	}
	if created.ID == "" {
		created.ID = idgen.New("areval")
	}
	if created.DurationMS < 0 {
		created.DurationMS = 0
	}
	if created.CandidateCount < 0 {
		created.CandidateCount = 0
	}
	if created.TriggeredCount < 0 {
		created.TriggeredCount = 0
	}

	m.evaluations[ruleID] = append(m.evaluations[ruleID], created)
	rule.LastEvaluatedAt = &evaluatedAt
	m.rules[ruleID] = rule
	return cloneAlertEvaluation(created), nil
}

func (m *MemoryAlertStore) ListAlertEvaluations(ruleID string, limit int) ([]alerts.Evaluation, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.rules[strings.TrimSpace(ruleID)]; !ok {
		return nil, alerts.ErrRuleNotFound
	}

	entries := m.evaluations[strings.TrimSpace(ruleID)]
	out := make([]alerts.Evaluation, 0, len(entries))
	for _, evaluation := range entries {
		out = append(out, cloneAlertEvaluation(evaluation))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].EvaluatedAt.After(out[j].EvaluatedAt)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func cloneAlertRule(input alerts.Rule) alerts.Rule {
	out := input
	out.Condition = cloneAnyMap(input.Condition)
	out.Labels = cloneMetadata(input.Labels)
	out.Metadata = cloneMetadata(input.Metadata)
	if len(input.Targets) > 0 {
		out.Targets = make([]alerts.RuleTarget, 0, len(input.Targets))
		for _, target := range input.Targets {
			out.Targets = append(out.Targets, alerts.RuleTarget{
				ID:        target.ID,
				RuleID:    target.RuleID,
				AssetID:   target.AssetID,
				GroupID:   target.GroupID,
				Selector:  cloneAnyMap(target.Selector),
				CreatedAt: target.CreatedAt,
			})
		}
	}
	if input.LastEvaluatedAt != nil {
		value := input.LastEvaluatedAt.UTC()
		out.LastEvaluatedAt = &value
	}
	return out
}

func cloneAlertEvaluation(input alerts.Evaluation) alerts.Evaluation {
	out := input
	out.Details = cloneAnyMap(input.Details)
	return out
}

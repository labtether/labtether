package persistence

import (
	"context"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/alerts"
)

func (m *MemoryAlertStore) ClaimAlertRuleEvaluation(ctx context.Context, id string, at time.Time) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := lockAlertEvaluation(ctx, &m.mu, false); err != nil {
		return false, err
	}
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	rule, ok := m.rules[strings.TrimSpace(id)]
	if !ok || rule.Status != alerts.RuleStatusActive || at.IsZero() {
		return false, nil
	}
	interval := rule.EvaluationIntervalSeconds
	if interval <= 0 {
		interval = alerts.DefaultEvaluationIntervalSeconds
	}
	if rule.LastAutoAttemptAt != nil && at.Before(rule.LastAutoAttemptAt.Add(time.Duration(interval)*time.Second)) {
		return false, nil
	}
	at = at.UTC()
	rule.LastAutoAttemptAt = &at
	m.rules[rule.ID] = rule
	return true, nil
}

func (m *MemoryAlertInstanceStore) AlertFingerprintTiming(ctx context.Context, ruleID, fingerprint string) (AlertFingerprintTiming, error) {
	if err := ctx.Err(); err != nil {
		return AlertFingerprintTiming{}, err
	}
	if err := lockAlertEvaluation(ctx, &m.mu, true); err != nil {
		return AlertFingerprintTiming{}, err
	}
	defer m.mu.RUnlock()
	if err := ctx.Err(); err != nil {
		return AlertFingerprintTiming{}, err
	}
	out := AlertFingerprintTiming{}
	for _, inst := range m.instances {
		if err := ctx.Err(); err != nil {
			return AlertFingerprintTiming{}, err
		}
		if inst.RuleID != strings.TrimSpace(ruleID) || inst.Fingerprint != strings.TrimSpace(fingerprint) || inst.FirstFiredAt == nil {
			continue
		}
		if out.LastFirstFiredAt == nil || inst.FirstFiredAt.After(*out.LastFirstFiredAt) {
			value := inst.FirstFiredAt.UTC()
			out.LastFirstFiredAt = &value
		}
		if inst.ResolvedAt != nil && (out.LastResolvedAt == nil || inst.ResolvedAt.After(*out.LastResolvedAt)) {
			value := inst.ResolvedAt.UTC()
			out.LastResolvedAt = &value
		}
	}
	return out, ctx.Err()
}

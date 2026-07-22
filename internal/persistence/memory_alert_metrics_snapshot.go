package persistence

import (
	"context"
	"fmt"
	"sort"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/telemetry"
)

// MemoryAlertMetricsSnapshotStore composes the two in-memory alert stores into
// the same exact-count, bounded-series snapshot contract used by Postgres.
type MemoryAlertMetricsSnapshotStore struct {
	rules     *MemoryAlertStore
	instances *MemoryAlertInstanceStore
}

func NewMemoryAlertMetricsSnapshotStore(rules *MemoryAlertStore, instances *MemoryAlertInstanceStore) *MemoryAlertMetricsSnapshotStore {
	return &MemoryAlertMetricsSnapshotStore{rules: rules, instances: instances}
}

func (m *MemoryAlertMetricsSnapshotStore) AlertMetricsSnapshot(ctx context.Context, maxRuleSeries int) (AlertMetricsSnapshot, error) {
	if ctx == nil {
		return AlertMetricsSnapshot{}, fmt.Errorf("alert metric snapshot context is required")
	}
	if maxRuleSeries <= 0 || maxRuleSeries > telemetry.MaxAlertRuleMetricSeries {
		return AlertMetricsSnapshot{}, ErrAlertMetricSnapshotLimitExceeded
	}
	if m == nil || m.rules == nil || m.instances == nil {
		return AlertMetricsSnapshot{}, fmt.Errorf("memory alert metric stores are required")
	}
	if err := ctx.Err(); err != nil {
		return AlertMetricsSnapshot{}, err
	}

	type activeRule struct {
		id   string
		name string
	}
	m.rules.mu.RLock()
	activeRules := make([]activeRule, 0, len(m.rules.rules))
	ruleScanCount := 0
	for _, rule := range m.rules.rules {
		ruleScanCount++
		if ruleScanCount%256 == 0 {
			if err := ctx.Err(); err != nil {
				m.rules.mu.RUnlock()
				return AlertMetricsSnapshot{}, err
			}
		}
		if rule.Status == alerts.RuleStatusActive {
			activeRules = append(activeRules, activeRule{id: rule.ID, name: rule.Name})
		}
	}
	sort.Slice(activeRules, func(i, j int) bool { return activeRules[i].id < activeRules[j].id })
	snapshot := AlertMetricsSnapshot{ActiveRuleCount: int64(len(activeRules))}
	if len(activeRules) > maxRuleSeries {
		activeRules = activeRules[:maxRuleSeries]
	}
	snapshot.RuleEvaluations = make([]AlertRuleMetricSnapshot, 0, len(activeRules))
	for i, rule := range activeRules {
		if i%128 == 0 {
			if err := ctx.Err(); err != nil {
				m.rules.mu.RUnlock()
				return AlertMetricsSnapshot{}, err
			}
		}
		evaluation, ok := m.rules.latestEvaluations[rule.id]
		if !ok {
			continue
		}
		snapshot.RuleEvaluations = append(snapshot.RuleEvaluations, AlertRuleMetricSnapshot{
			RuleID:     rule.id,
			RuleName:   rule.name,
			DurationMS: evaluation.DurationMS,
		})
	}
	m.rules.mu.RUnlock()

	if err := ctx.Err(); err != nil {
		return AlertMetricsSnapshot{}, err
	}
	m.instances.mu.RLock()
	instanceScanCount := 0
	for _, instance := range m.instances.instances {
		instanceScanCount++
		if instanceScanCount%256 == 0 {
			if err := ctx.Err(); err != nil {
				m.instances.mu.RUnlock()
				return AlertMetricsSnapshot{}, err
			}
		}
		if instance.Status == alerts.InstanceStatusFiring {
			snapshot.FiringInstanceCount++
		}
	}
	m.instances.mu.RUnlock()
	if err := ctx.Err(); err != nil {
		return AlertMetricsSnapshot{}, err
	}
	return snapshot, nil
}

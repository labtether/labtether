package persistence

import (
	"context"
	"sort"

	"github.com/labtether/labtether/internal/alerts"
)

func (m *MemoryAlertInstanceStore) ReadFiringAlertInstances(ctx context.Context, ruleIDs []string, visit func(alerts.AlertInstance) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	selected := make(map[string]bool, len(ruleIDs))
	for _, id := range ruleIDs {
		selected[id] = true
	}
	if err := lockAlertEvaluation(ctx, &m.mu, true); err != nil {
		return err
	}
	defer m.mu.RUnlock()
	ids := make([]string, 0)
	for id, row := range m.instances {
		if err := ctx.Err(); err != nil {
			return err
		}
		if row.Status == alerts.InstanceStatusFiring && (ruleIDs == nil || selected[row.RuleID]) {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := visit(cloneAlertInstance(m.instances[id])); err != nil {
			return err
		}
	}
	return ctx.Err()
}

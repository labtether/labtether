package persistence

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/alerts"
)

func (m *MemoryAlertInstanceStore) FireAlertInstance(ctx context.Context, id string, scope alerts.FiringScope) (alerts.AlertInstance, error) {
	normalized, err := alerts.NormalizeFiringScope(scope)
	if err != nil {
		return alerts.AlertInstance{}, err
	}
	if err = ctx.Err(); err != nil {
		return alerts.AlertInstance{}, err
	}
	if err := lockAlertEvaluation(ctx, &m.mu, false); err != nil {
		return alerts.AlertInstance{}, err
	}
	defer m.mu.Unlock()
	if err = ctx.Err(); err != nil {
		return alerts.AlertInstance{}, err
	}
	inst, ok := m.instances[strings.TrimSpace(id)]
	if !ok {
		return alerts.AlertInstance{}, errors.New("alert instance not found")
	}
	if inst.Status == alerts.InstanceStatusFiring || inst.Status == alerts.InstanceStatusAcknowledged {
		return cloneAlertInstance(inst), nil
	}
	if inst.Status != alerts.InstanceStatusPending || inst.FirstFiredAt != nil || inst.FiringScope != nil {
		return alerts.AlertInstance{}, errors.New("alert instance is not a fresh pending instance")
	}
	now := time.Now().UTC()
	inst.Status, inst.FirstFiredAt, inst.LastFiredAt, inst.UpdatedAt = alerts.InstanceStatusFiring, &now, now, now
	inst.FiringScope = &normalized
	m.instances[inst.ID] = cloneAlertInstance(inst)
	return cloneAlertInstance(inst), nil
}

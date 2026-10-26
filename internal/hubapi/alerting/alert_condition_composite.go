package alerting

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/persistence"
)

const compositeReadTimeout = 5 * time.Second

func (d *Deps) evaluateCompositeOutcome(ctx context.Context, rule alerts.Rule, prefetched []alerts.AlertInstance) (alertConditionOutcome, error) {
	outcome := newAlertConditionOutcome()
	raw, ok := rule.Condition["rule_ids"].([]interface{})
	if !ok || len(raw) == 0 {
		return outcome, nil
	}
	matched := map[string]bool{}
	for _, value := range raw {
		if id, ok := value.(string); ok && id != "" {
			matched[id] = false
		}
	}
	if len(matched) == 0 {
		return outcome, nil
	}
	ctx, cancel := context.WithTimeout(ctx, compositeReadTimeout)
	defer cancel()
	visit := func(inst alerts.AlertInstance) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, selected := matched[inst.RuleID]; selected && inst.Status == alerts.InstanceStatusFiring {
			matched[inst.RuleID] = true
			if inst.FirstFiredAt != nil {
				outcome.includeScope(inst.FiringScope)
			}
		}
		return nil
	}
	if prefetched != nil {
		for _, inst := range prefetched {
			if err := visit(inst); err != nil {
				return alertConditionOutcome{}, err
			}
		}
	} else {
		store, ok := d.AlertInstanceStore.(persistence.AlertFiringSnapshotStore)
		if !ok {
			return alertConditionOutcome{}, fmt.Errorf("complete composite alert snapshot unavailable")
		}
		ids := make([]string, 0, len(matched))
		for id := range matched {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		if err := store.ReadFiringAlertInstances(ctx, ids, visit); err != nil {
			return alertConditionOutcome{}, fmt.Errorf("composite firing snapshot: %w", err)
		}
	}
	if err := ctx.Err(); err != nil {
		return alertConditionOutcome{}, err
	}
	count := 0
	for _, found := range matched {
		if found {
			count++
		}
	}
	outcome.Triggered = (rule.Condition["operator"] == "or" && count > 0) || (rule.Condition["operator"] != "or" && count == len(matched))
	if !outcome.Triggered {
		return newAlertConditionOutcome(), nil
	}
	return outcome.result()
}

func (d *Deps) prefetchFiringInstances(ctx context.Context) []alerts.AlertInstance {
	store, ok := d.AlertInstanceStore.(persistence.AlertFiringSnapshotStore)
	if !ok {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, compositeReadTimeout)
	defer cancel()
	// Keep the shared fast path bounded. A full page is incomplete, so a
	// composite must instead read all of its exact rules in one snapshot.
	const limit = 500
	full := errors.New("firing prefetch incomplete")
	instances := make([]alerts.AlertInstance, 0)
	err := store.ReadFiringAlertInstances(ctx, nil, func(inst alerts.AlertInstance) error {
		instances = append(instances, inst)
		if len(instances) == limit {
			return full
		}
		return nil
	})
	if err != nil {
		if !errors.Is(err, full) {
			log.Printf("alert evaluator: failed to prefetch firing instances: %v", err)
		}
		return nil
	}
	return instances
}

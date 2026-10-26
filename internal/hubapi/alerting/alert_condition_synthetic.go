package alerting

import (
	"context"
	"fmt"
	"github.com/labtether/labtether/internal/persistence"

	"github.com/labtether/labtether/internal/alerts"
)

func (d *Deps) evaluateSyntheticOutcome(ctx context.Context, rule alerts.Rule) (alertConditionOutcome, error) {
	outcome := newAlertConditionOutcome()
	if err := ctx.Err(); err != nil {
		return outcome, err
	}
	consecutiveFailures, err := syntheticFailureCount(rule.Condition)
	if err != nil {
		return outcome, err
	}
	if d.SyntheticStore == nil {
		return outcome, nil
	}

	checkID, _ := rule.Condition["check_id"].(string)
	if checkID == "" {
		return outcome, nil
	}

	store, ok := d.SyntheticStore.(persistence.AlertSyntheticContextStore)
	if !ok {
		return outcome, fmt.Errorf("context-aware synthetic query unavailable")
	}
	check, found, err := store.GetSyntheticCheckContext(ctx, checkID)
	if err != nil {
		return outcome, fmt.Errorf("synthetic check lookup: %w", err)
	}
	if !found || check.ID != checkID {
		return outcome, nil
	}
	results, err := store.ListSyntheticResultsContext(ctx, checkID, consecutiveFailures)
	if err != nil {
		return alertConditionOutcome{}, fmt.Errorf("synthetic results query for check %s: %w", checkID, err)
	}

	if len(results) < consecutiveFailures {
		return outcome, nil
	}

	// Check if the most recent N results are all failures
	for _, r := range results {
		if r.CheckID != checkID || r.Status == "ok" {
			return outcome, nil
		}
	}

	outcome.Triggered = true
	outcome.Scope.SyntheticCheckIDs = []string{check.ID}
	if check.ServiceID != "" && d.ResolveSyntheticServiceAsset != nil {
		asset, known, lookupErr := d.ResolveSyntheticServiceAsset(ctx, check.ServiceID)
		if lookupErr == nil && known {
			outcome.witnessAsset(asset)
		}
	}
	if err := ctx.Err(); err != nil {
		return alertConditionOutcome{}, err
	}
	return outcome.result()
}

func (d *Deps) evaluateSyntheticCheck(ctx context.Context, rule alerts.Rule) (bool, error) {
	outcome, err := d.evaluateSyntheticOutcome(ctx, rule)
	return outcome.Triggered, err
}

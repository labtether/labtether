package alerting

import (
	"context"
	"fmt"
	"github.com/labtether/labtether/internal/persistence"
	"log"
	"time"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/assets"
)

type AlertEvaluationPrefetch struct {
	Assets               []assets.Asset
	CapabilityIDsByAsset map[string][]string
	FiringInstances      []alerts.AlertInstance
	Suppression          *alertSuppressionPrefetch
	assetSnapshot        *alertAssetSnapshot
}

func (d *Deps) RunAlertEvaluator(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	d.runAlertEvaluator(ctx, ticker.C)
}

func (d *Deps) runAlertEvaluator(ctx context.Context, ticks <-chan time.Time) {
	log.Printf("alert evaluator started (interval=30s)")

	for {
		select {
		case <-ctx.Done():
			log.Printf("alert evaluator stopped")
			return
		case <-ticks:
			d.evaluateAlertRules(ctx)
		}
	}
}

func (d *Deps) evaluateAlertRules(ctx context.Context) {
	d.evaluateAlertRulesWithClock(ctx, time.Now)
}

func (d *Deps) evaluateAlertRulesWithClock(ctx context.Context, now func() time.Time) {
	d.evaluateAlertRuleSlice(ctx, now)
}

func hasCompositeAlertRules(rules []alerts.Rule) bool {
	for _, rule := range rules {
		if rule.Kind == alerts.RuleKindComposite {
			return true
		}
	}
	return false
}

// evaluateRuleCondition reads the same condition for background evaluation and
// manual previews. It never records history, changes instances or sends notices.
func (d *Deps) evaluateRuleCondition(ctx context.Context, rule alerts.Rule, prefetch *AlertEvaluationPrefetch) (bool, error) {
	outcome, err := d.evaluateRuleOutcome(ctx, rule, prefetch)
	return outcome.Triggered, err
}

func (d *Deps) evaluateRuleOutcome(ctx context.Context, rule alerts.Rule, prefetch *AlertEvaluationPrefetch) (alertConditionOutcome, error) {
	if err := ctx.Err(); err != nil {
		return alertConditionOutcome{}, err
	}
	var prefetchedAssets []assets.Asset
	var prefetchedCapabilities map[string][]string
	var prefetchedFiringInstances []alerts.AlertInstance
	if prefetch != nil {
		var err error
		prefetchedAssets, err = d.prefetchedRuleAssets(ctx, rule, prefetch)
		if err != nil {
			return alertConditionOutcome{}, err
		}
		prefetchedCapabilities = prefetch.CapabilityIDsByAsset
		prefetchedFiringInstances = prefetch.FiringInstances
	}

	switch rule.Kind {
	case alerts.RuleKindHeartbeatStale:
		return d.evaluateHeartbeatStaleOutcome(ctx, rule, prefetchedAssets, prefetchedCapabilities)
	case alerts.RuleKindMetricThreshold:
		return d.evaluateMetricThresholdOutcome(ctx, rule, prefetchedAssets, prefetchedCapabilities)
	case alerts.RuleKindMetricDeadman:
		return d.evaluateMetricDeadmanOutcome(ctx, rule, prefetchedAssets, prefetchedCapabilities)
	case alerts.RuleKindLogPattern:
		return d.evaluateLogPatternOutcome(ctx, rule, prefetchedAssets, prefetchedCapabilities)
	case alerts.RuleKindSyntheticCheck:
		return d.evaluateSyntheticOutcome(ctx, rule)
	case alerts.RuleKindComposite:
		return d.evaluateCompositeOutcome(ctx, rule, prefetchedFiringInstances)
	default:
		return newAlertConditionOutcome(), nil
	}
}

func (d *Deps) EvaluateSingleRule(ctx context.Context, rule alerts.Rule, prefetch *AlertEvaluationPrefetch) {
	d.evaluateSingleRuleAt(ctx, rule, prefetch, time.Now().UTC())
}

func (d *Deps) evaluateSingleRuleAt(ctx context.Context, rule alerts.Rule, prefetch *AlertEvaluationPrefetch, at time.Time) {
	start := time.Now()
	outcome, evalErr := d.evaluateRuleOutcome(ctx, rule, prefetch)
	if ctx.Err() != nil {
		return
	}
	candidateCount := len(rule.Targets)
	var prefetchedSuppression *alertSuppressionPrefetch
	if prefetch != nil {
		prefetchedSuppression = prefetch.Suppression
	}

	status := alerts.EvaluationStatusOK
	triggeredCount := 0
	errMsg := ""
	if evalErr != nil {
		status = alerts.EvaluationStatusError
		errMsg = evalErr.Error()
	} else if outcome.Triggered {
		status = alerts.EvaluationStatusTriggered
		triggeredCount = 1
		d.fireOrRefireAlertWithScopeAt(ctx, rule, prefetchedSuppression, at, outcome.Scope)
	} else {
		d.resolveStaleInstancesContext(ctx, rule, prefetchedSuppression)
	}

	_, err := d.recordAlertEvaluationContext(ctx, rule.ID, alerts.Evaluation{
		Status:         status,
		EvaluatedAt:    time.Now().UTC(),
		DurationMS:     int(time.Since(start).Milliseconds()),
		CandidateCount: candidateCount,
		TriggeredCount: triggeredCount,
		Error:          errMsg,
	})
	if err != nil && ctx.Err() == nil {
		log.Printf("alert evaluator: failed to record evaluation: %v", err)
	}
}

func (d *Deps) recordAlertEvaluationContext(ctx context.Context, ruleID string, evaluation alerts.Evaluation) (alerts.Evaluation, error) {
	if err := ctx.Err(); err != nil {
		return alerts.Evaluation{}, err
	}
	store, ok := d.AlertStore.(persistence.AlertEvaluationContextStore)
	if !ok {
		return alerts.Evaluation{}, fmt.Errorf("context-aware alert history unavailable")
	}
	return store.RecordAlertEvaluationContext(ctx, ruleID, evaluation)
}

package alerting

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/persistence"
)

type AlertEvaluationPrefetch struct {
	Assets               []assets.Asset
	CapabilityIDsByAsset map[string][]string
	FiringRuleIDs        map[string]struct{}
	Suppression          *alertSuppressionPrefetch
}

func (d *Deps) RunAlertEvaluator(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	log.Printf("alert evaluator started (interval=30s)")

	for {
		select {
		case <-ctx.Done():
			log.Printf("alert evaluator stopped")
			return
		case <-ticker.C:
			d.evaluateAlertRules(ctx)
		}
	}
}

func (d *Deps) evaluateAlertRules(ctx context.Context) {
	if d.AlertStore == nil || d.AlertInstanceStore == nil {
		return
	}

	rules, err := d.AlertStore.ListAlertRules(persistence.AlertRuleFilter{
		Limit:  500,
		Status: alerts.RuleStatusActive,
	})
	if err != nil {
		log.Printf("alert evaluator: failed to list rules: %v", err)
		return
	}

	prefetch := AlertEvaluationPrefetch{
		Assets:               nil,
		CapabilityIDsByAsset: map[string][]string{},
		FiringRuleIDs:        nil,
	}

	if d.AssetStore != nil {
		prefetch.Assets, err = d.AssetStore.ListAssets()
		if err != nil {
			log.Printf("alert evaluator: failed to prefetch assets: %v", err)
			prefetch.Assets = nil
		}
	}

	if d.CanonicalStore != nil && len(prefetch.Assets) > 0 {
		prefetch.CapabilityIDsByAsset = d.prefetchAlertCapabilityIDsByAsset(prefetch.Assets)
	}

	if hasCompositeAlertRules(rules) {
		prefetch.FiringRuleIDs = d.prefetchFiringRuleIDs(5000)
	}
	prefetch.Suppression = d.newAlertSuppressionPrefetch()

	for _, rule := range rules {
		select {
		case <-ctx.Done():
			return
		default:
		}
		d.EvaluateSingleRule(ctx, rule, &prefetch)
	}
}

func hasCompositeAlertRules(rules []alerts.Rule) bool {
	for _, rule := range rules {
		if rule.Kind == alerts.RuleKindComposite {
			return true
		}
	}
	return false
}

func (d *Deps) prefetchAlertCapabilityIDsByAsset(prefetchedAssets []assets.Asset) map[string][]string {
	out := make(map[string][]string, len(prefetchedAssets))
	if d.CanonicalStore == nil || len(prefetchedAssets) == 0 {
		return out
	}

	assetIDs := make(map[string]struct{}, len(prefetchedAssets))
	for _, assetEntry := range prefetchedAssets {
		assetID := strings.TrimSpace(assetEntry.ID)
		if assetID == "" {
			continue
		}
		assetIDs[assetID] = struct{}{}
	}
	if len(assetIDs) == 0 {
		return out
	}

	limit := len(assetIDs) + 256
	if limit < 500 {
		limit = 500
	}
	if limit > 5000 {
		limit = 5000
	}

	capabilitySets, err := d.CanonicalStore.ListCapabilitySets(limit)
	if err != nil {
		log.Printf("alert evaluator: failed to prefetch capability sets: %v", err)
		return out
	}

	for _, capabilitySet := range capabilitySets {
		if !strings.EqualFold(strings.TrimSpace(capabilitySet.SubjectType), "resource") {
			continue
		}
		subjectID := strings.TrimSpace(capabilitySet.SubjectID)
		if _, ok := assetIDs[subjectID]; !ok {
			continue
		}
		out[subjectID] = d.capabilityIDsFromSet(capabilitySet)
	}
	return out
}

func (d *Deps) prefetchFiringRuleIDs(limit int) map[string]struct{} {
	if d.AlertInstanceStore == nil {
		return nil
	}
	if limit <= 0 {
		limit = 1000
	}

	instances, err := d.AlertInstanceStore.ListAlertInstances(persistence.AlertInstanceFilter{
		Status: alerts.InstanceStatusFiring,
		Limit:  limit,
	})
	if err != nil {
		log.Printf("alert evaluator: failed to prefetch firing instances: %v", err)
		return nil
	}

	firingRuleIDs := make(map[string]struct{}, len(instances))
	for _, instance := range instances {
		ruleID := strings.TrimSpace(instance.RuleID)
		if ruleID == "" {
			continue
		}
		firingRuleIDs[ruleID] = struct{}{}
	}
	return firingRuleIDs
}

func (d *Deps) EvaluateSingleRule(ctx context.Context, rule alerts.Rule, prefetch *AlertEvaluationPrefetch) {
	start := time.Now()
	triggered := false
	candidateCount := len(rule.Targets)
	var evalErr error

	var prefetchedAssets []assets.Asset
	var prefetchedCapabilities map[string][]string
	var prefetchedFiringRuleIDs map[string]struct{}
	var prefetchedSuppression *alertSuppressionPrefetch
	if prefetch != nil {
		prefetchedAssets = prefetch.Assets
		prefetchedCapabilities = prefetch.CapabilityIDsByAsset
		prefetchedFiringRuleIDs = prefetch.FiringRuleIDs
		prefetchedSuppression = prefetch.Suppression
	}

	switch rule.Kind {
	case alerts.RuleKindHeartbeatStale:
		triggered, evalErr = d.EvaluateHeartbeatStaleWithPrefetch(rule, prefetchedAssets, prefetchedCapabilities)
	case alerts.RuleKindMetricThreshold:
		triggered, evalErr = d.EvaluateMetricThresholdWithPrefetch(rule, prefetchedAssets, prefetchedCapabilities)
	case alerts.RuleKindMetricDeadman:
		triggered, evalErr = d.EvaluateMetricDeadmanWithPrefetch(rule, prefetchedAssets, prefetchedCapabilities)
	case alerts.RuleKindLogPattern:
		triggered, evalErr = d.EvaluateLogPatternWithPrefetch(rule, prefetchedAssets, prefetchedCapabilities)
	case alerts.RuleKindSyntheticCheck:
		triggered, evalErr = d.evaluateSyntheticCheck(ctx, rule)
	case alerts.RuleKindComposite:
		triggered, evalErr = d.evaluateCompositeWithPrefetchedFiring(rule, prefetchedFiringRuleIDs)
	default:
		triggered = false
	}

	status := alerts.EvaluationStatusOK
	triggeredCount := 0
	errMsg := ""
	if evalErr != nil {
		status = alerts.EvaluationStatusError
		errMsg = evalErr.Error()
	} else if triggered {
		status = alerts.EvaluationStatusTriggered
		triggeredCount = 1
		d.fireOrRefireAlert(rule, prefetchedSuppression)
	} else {
		d.resolveStaleInstances(rule)
	}

	_, _ = d.AlertStore.RecordAlertEvaluation(rule.ID, alerts.Evaluation{
		Status:         status,
		EvaluatedAt:    time.Now().UTC(),
		DurationMS:     int(time.Since(start).Milliseconds()),
		CandidateCount: candidateCount,
		TriggeredCount: triggeredCount,
		Error:          errMsg,
	})
}

// evaluateCompositeWithPrefetchedFiring evaluates a composite alert rule by checking
// whether its referenced sub-rules currently have firing alert instances.
//
// The rule's Condition map must contain:
//   - "rule_ids": []interface{} of sub-rule ID strings to evaluate.
//   - "operator": "and" (default) | "or" — how to combine sub-rule results.
//
// "and" requires ALL sub-rules to have at least one firing instance.
// "or"  requires ANY sub-rule to have at least one firing instance.
// Sub-rule IDs that yield a store error are skipped (treated as not firing).
func (d *Deps) evaluateCompositeWithPrefetchedFiring(
	rule alerts.Rule,
	firingRuleIDs map[string]struct{},
) (bool, error) {
	raw, ok := rule.Condition["rule_ids"]
	if !ok {
		return false, nil
	}
	rawSlice, ok := raw.([]interface{})
	if !ok || len(rawSlice) == 0 {
		return false, nil
	}

	subRuleIDs := make([]string, 0, len(rawSlice))
	for _, v := range rawSlice {
		id, ok := v.(string)
		if !ok || id == "" {
			continue
		}
		subRuleIDs = append(subRuleIDs, id)
	}
	if len(subRuleIDs) == 0 {
		return false, nil
	}

	operator := "and"
	if opRaw, ok := rule.Condition["operator"]; ok {
		if opStr, ok := opRaw.(string); ok && opStr == "or" {
			operator = "or"
		}
	}

	for _, subRuleID := range subRuleIDs {
		hasFiring, err := d.compositeSubRuleHasFiring(subRuleID, firingRuleIDs)
		if err != nil {
			log.Printf("alert evaluator: composite rule %s: failed to query sub-rule %s: %v", rule.ID, subRuleID, err)
			if operator == "and" {
				return false, nil
			}
			continue
		}

		if operator == "or" && hasFiring {
			return true, nil
		}
		if operator == "and" && !hasFiring {
			return false, nil
		}
	}

	return operator == "and", nil
}

func (d *Deps) compositeSubRuleHasFiring(subRuleID string, firingRuleIDs map[string]struct{}) (bool, error) {
	subRuleID = strings.TrimSpace(subRuleID)
	if subRuleID == "" {
		return false, nil
	}

	if firingRuleIDs != nil {
		_, ok := firingRuleIDs[subRuleID]
		return ok, nil
	}

	instances, err := d.AlertInstanceStore.ListAlertInstances(persistence.AlertInstanceFilter{
		RuleID: subRuleID,
		Status: alerts.InstanceStatusFiring,
		Limit:  1,
	})
	if err != nil {
		return false, err
	}
	return len(instances) > 0, nil
}

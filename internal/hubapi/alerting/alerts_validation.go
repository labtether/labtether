package alerting

import (
	"errors"
	"fmt"
	"strings"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/groups"
)

func NormalizeCreateAlertRuleRequest(req *alerts.CreateRuleRequest) {
	req.Name = strings.TrimSpace(req.Name)
	req.Description = strings.TrimSpace(req.Description)
	req.Status = strings.TrimSpace(req.Status)
	req.Kind = strings.TrimSpace(req.Kind)
	req.Severity = strings.TrimSpace(req.Severity)
	req.TargetScope = strings.TrimSpace(req.TargetScope)
	req.CreatedBy = strings.TrimSpace(req.CreatedBy)
	req.Labels = cloneMetadata(req.Labels)
	req.Metadata = cloneMetadata(req.Metadata)
	for idx := range req.Targets {
		req.Targets[idx].AssetID = strings.TrimSpace(req.Targets[idx].AssetID)
		req.Targets[idx].GroupID = strings.TrimSpace(req.Targets[idx].GroupID)
		req.Targets[idx].Selector = cloneAnyMap(req.Targets[idx].Selector)
	}
}

func ValidateCreateAlertRuleRequest(req alerts.CreateRuleRequest) error {
	if req.Name == "" {
		return errors.New("name is required")
	}
	if err := validateMaxLen("name", req.Name, MaxAlertRuleNameLength); err != nil {
		return err
	}
	if err := validateMaxLen("description", req.Description, MaxAlertDescriptionLen); err != nil {
		return err
	}
	if req.Kind == "" {
		return errors.New("kind is required")
	}
	if alerts.NormalizeRuleKind(req.Kind) == "" {
		return errors.New("kind must be metric_threshold, metric_deadman, heartbeat_stale, log_pattern, composite, or synthetic_check")
	}
	if req.Severity == "" {
		return errors.New("severity is required")
	}
	if alerts.NormalizeSeverity(req.Severity) == "" {
		return errors.New("severity must be critical, high, medium, or low")
	}
	if req.TargetScope == "" {
		return errors.New("target_scope is required")
	}
	if alerts.NormalizeTargetScope(req.TargetScope) == "" {
		return errors.New("target_scope must be asset, group, or global")
	}
	if req.Status != "" && alerts.NormalizeRuleStatus(req.Status) == "" {
		return errors.New("status must be active or paused")
	}
	if req.CooldownSeconds < 0 {
		return errors.New("cooldown_seconds must be >= 0")
	}
	if req.ReopenAfterSeconds < 0 {
		return errors.New("reopen_after_seconds must be >= 0")
	}
	if req.EvaluationIntervalSeconds < 0 {
		return errors.New("evaluation_interval_seconds must be >= 0")
	}
	if req.WindowSeconds < 0 {
		return errors.New("window_seconds must be >= 0")
	}
	if len(req.Condition) == 0 {
		return errors.New("condition is required")
	}
	if len(req.Targets) > MaxAlertTargetCount {
		return errors.New("too many alert rule targets")
	}
	if req.TargetScope != alerts.TargetScopeGlobal && len(req.Targets) == 0 {
		return errors.New("targets are required when target_scope is not global")
	}
	for _, target := range req.Targets {
		if alerts.TargetReferenceCount(target) != 1 {
			return errors.New("each alert target must provide exactly one of asset_id, group_id, or selector")
		}
	}
	return nil
}

func NormalizeUpdateAlertRuleRequest(req *alerts.UpdateRuleRequest) {
	req.Name = trimStringPtr(req.Name)
	req.Description = trimStringPtr(req.Description)
	req.Status = trimStringPtr(req.Status)
	req.Severity = trimStringPtr(req.Severity)
	if req.Condition != nil {
		normalized := cloneAnyMap(*req.Condition)
		req.Condition = &normalized
	}
	if req.Labels != nil {
		normalized := cloneMetadata(*req.Labels)
		req.Labels = &normalized
	}
	if req.Metadata != nil {
		normalized := cloneMetadata(*req.Metadata)
		req.Metadata = &normalized
	}
	if req.Targets != nil {
		targets := append([]alerts.RuleTargetInput(nil), (*req.Targets)...)
		for idx := range targets {
			targets[idx].AssetID = strings.TrimSpace(targets[idx].AssetID)
			targets[idx].GroupID = strings.TrimSpace(targets[idx].GroupID)
			targets[idx].Selector = cloneAnyMap(targets[idx].Selector)
		}
		req.Targets = &targets
	}
}

func ValidateUpdateAlertRuleRequest(existing alerts.Rule, req alerts.UpdateRuleRequest) error {
	if req.Name != nil {
		if *req.Name == "" {
			return errors.New("name cannot be empty")
		}
		if err := validateMaxLen("name", *req.Name, MaxAlertRuleNameLength); err != nil {
			return err
		}
	}
	if req.Description != nil {
		if err := validateMaxLen("description", *req.Description, MaxAlertDescriptionLen); err != nil {
			return err
		}
	}
	if req.Status != nil {
		if alerts.NormalizeRuleStatus(*req.Status) == "" {
			return errors.New("status must be active or paused")
		}
	}
	if req.Severity != nil {
		if alerts.NormalizeSeverity(*req.Severity) == "" {
			return errors.New("severity must be critical, high, medium, or low")
		}
	}
	if req.CooldownSeconds != nil && *req.CooldownSeconds < 0 {
		return errors.New("cooldown_seconds must be >= 0")
	}
	if req.ReopenAfterSeconds != nil && *req.ReopenAfterSeconds < 0 {
		return errors.New("reopen_after_seconds must be >= 0")
	}
	if req.EvaluationIntervalSeconds != nil && *req.EvaluationIntervalSeconds < 0 {
		return errors.New("evaluation_interval_seconds must be >= 0")
	}
	if req.WindowSeconds != nil && *req.WindowSeconds < 0 {
		return errors.New("window_seconds must be >= 0")
	}
	if req.Targets != nil {
		if len(*req.Targets) > MaxAlertTargetCount {
			return errors.New("too many alert rule targets")
		}
		if existing.TargetScope != alerts.TargetScopeGlobal && len(*req.Targets) == 0 {
			return errors.New("targets are required when target_scope is not global")
		}
		for _, target := range *req.Targets {
			if alerts.TargetReferenceCount(target) != 1 {
				return errors.New("each alert target must provide exactly one of asset_id, group_id, or selector")
			}
		}
	}
	return nil
}

func (d *Deps) ValidateAlertRuleTargets(targets []alerts.RuleTargetInput) error {
	for _, target := range targets {
		if alerts.TargetReferenceCount(target) != 1 {
			return errors.New("each alert target must provide exactly one of asset_id, group_id, or selector")
		}
		if target.AssetID != "" {
			if _, ok, err := d.AssetStore.GetAsset(target.AssetID); err != nil {
				return err
			} else if !ok {
				return fmt.Errorf("asset not found: %s", target.AssetID)
			}
		}
		if target.GroupID != "" {
			if _, ok, err := d.GroupStore.GetGroup(target.GroupID); err != nil {
				return err
			} else if !ok {
				return groups.ErrGroupNotFound
			}
		}
		if len(target.Selector) > 0 {
			for key := range target.Selector {
				if strings.TrimSpace(key) == "" {
					return errors.New("selector key cannot be empty")
				}
			}
			if err := ValidateNoDeprecatedCanonicalPredicateKeys(target.Selector, "selector"); err != nil {
				return err
			}
		}
	}
	return nil
}

package alerting

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/incidents"
)

func writeAssetScopeForbidden(w http.ResponseWriter, message string) {
	apiv2.WriteError(w, http.StatusForbidden, "asset_forbidden", message)
}

func denyAssetRestrictedGlobal(w http.ResponseWriter, r *http.Request, object string) bool {
	if !shared.HasAssetRestriction(r.Context()) {
		return false
	}
	writeAssetScopeForbidden(w, "asset-restricted api keys cannot access global "+object)
	return true
}

func (d *Deps) accessibleGroupIDs(ctx context.Context) (map[string]struct{}, error) {
	if !shared.HasAssetRestriction(ctx) {
		return nil, nil
	}
	if d.GroupStore == nil || d.AssetStore == nil {
		return nil, fmt.Errorf("asset authorization stores unavailable")
	}
	groupList, err := d.GroupStore.ListGroups()
	if err != nil {
		return nil, err
	}
	assetList, err := d.AssetStore.ListAssets()
	if err != nil {
		return nil, err
	}
	return shared.AccessibleGroupIDs(ctx, groupList, assetList), nil
}

func alertRuleAllowed(ctx context.Context, rule alerts.Rule, groupAccess map[string]struct{}) bool {
	if !shared.HasAssetRestriction(ctx) {
		return true
	}
	if len(rule.Targets) == 0 {
		return false
	}
	for _, target := range rule.Targets {
		assetID := strings.TrimSpace(target.AssetID)
		groupID := strings.TrimSpace(target.GroupID)
		switch {
		case assetID != "" && groupID == "" && len(target.Selector) == 0:
			if !apiv2.AssetCheckContext(ctx, assetID) {
				return false
			}
		case groupID != "" && assetID == "" && len(target.Selector) == 0:
			if _, ok := groupAccess[groupID]; !ok {
				return false
			}
		default:
			// Selector/global/malformed targets cannot be proven to stay within a
			// concrete allowlist, so restricted principals fail closed.
			return false
		}
	}
	return true
}

func alertRuleInputsAllowed(ctx context.Context, targets []alerts.RuleTargetInput, groupAccess map[string]struct{}) bool {
	converted := make([]alerts.RuleTarget, 0, len(targets))
	for _, target := range targets {
		converted = append(converted, alerts.RuleTarget{
			AssetID:  target.AssetID,
			GroupID:  target.GroupID,
			Selector: target.Selector,
		})
	}
	return alertRuleAllowed(ctx, alerts.Rule{Targets: converted}, groupAccess)
}

func (d *Deps) requireAlertRuleAccess(w http.ResponseWriter, r *http.Request, rule alerts.Rule) bool {
	if !shared.HasAssetRestriction(r.Context()) {
		return true
	}
	groupAccess, err := d.accessibleGroupIDs(r.Context())
	if err != nil {
		writeAssetScopeForbidden(w, "unable to prove alert rule asset scope")
		return false
	}
	if alertRuleAllowed(r.Context(), rule, groupAccess) {
		return true
	}
	writeAssetScopeForbidden(w, "api key does not have access to every asset targeted by this alert rule")
	return false
}

func (d *Deps) alertInstanceAllowed(ctx context.Context, instance alerts.AlertInstance, groupAccess map[string]struct{}) (bool, error) {
	if d.AlertStore == nil {
		return false, fmt.Errorf("alert rule authorization store unavailable")
	}
	rule, ok, err := d.AlertStore.GetAlertRule(instance.RuleID)
	if err != nil || !ok {
		return false, err
	}
	if !alertRuleAllowed(ctx, rule, groupAccess) {
		return false, nil
	}
	for _, label := range []string{"asset_id", "target_asset_id", "source_asset_id"} {
		if assetID := strings.TrimSpace(instance.Labels[label]); assetID != "" && !apiv2.AssetCheckContext(ctx, assetID) {
			return false, nil
		}
	}
	return true, nil
}

// AlertInstanceAllowedForAccess applies the same asset-allowlist policy used by
// the HTTP handlers to internal callers such as the MCP bridge.
func (d *Deps) AlertInstanceAllowedForAccess(ctx context.Context, instance alerts.AlertInstance) (bool, error) {
	if !shared.HasAssetRestriction(ctx) {
		return true, nil
	}
	groupAccess, err := d.accessibleGroupIDs(ctx)
	if err != nil {
		return false, err
	}
	return d.alertInstanceAllowed(ctx, instance, groupAccess)
}

// FilterAlertInstancesForAccess removes instances whose complete rule and
// concrete asset scope cannot be proven to fit the caller's allowlist.
func (d *Deps) FilterAlertInstancesForAccess(ctx context.Context, instances []alerts.AlertInstance) ([]alerts.AlertInstance, error) {
	if !shared.HasAssetRestriction(ctx) {
		return instances, nil
	}
	groupAccess, err := d.accessibleGroupIDs(ctx)
	if err != nil {
		return nil, err
	}
	filtered := make([]alerts.AlertInstance, 0, len(instances))
	for _, instance := range instances {
		allowed, checkErr := d.alertInstanceAllowed(ctx, instance, groupAccess)
		if checkErr != nil {
			return nil, checkErr
		}
		if allowed {
			filtered = append(filtered, instance)
		}
	}
	return filtered, nil
}

func (d *Deps) incidentAllowed(ctx context.Context, incident incidents.Incident, groupAccess map[string]struct{}) (bool, error) {
	if !shared.HasAssetRestriction(ctx) {
		return true, nil
	}
	hasConcreteScope := false
	if assetID := strings.TrimSpace(incident.PrimaryAssetID); assetID != "" {
		hasConcreteScope = true
		if !apiv2.AssetCheckContext(ctx, assetID) {
			return false, nil
		}
	}
	if groupID := strings.TrimSpace(incident.GroupID); groupID != "" {
		hasConcreteScope = true
		if _, ok := groupAccess[groupID]; !ok {
			return false, nil
		}
	}
	if d.DependencyStore == nil {
		return false, fmt.Errorf("incident asset authorization store unavailable")
	}
	linked, err := d.DependencyStore.ListIncidentAssets(incident.ID, 10000)
	if err != nil {
		return false, err
	}
	for _, link := range linked {
		hasConcreteScope = true
		if !apiv2.AssetCheckContext(ctx, link.AssetID) {
			return false, nil
		}
	}
	if d.IncidentStore == nil {
		return false, fmt.Errorf("incident alert authorization store unavailable")
	}
	alertLinks, err := d.IncidentStore.ListIncidentAlertLinks(incident.ID, 10000)
	if err != nil {
		return false, err
	}
	for _, link := range alertLinks {
		allowed, err := d.incidentAlertLinkAllowed(ctx, link, groupAccess)
		if err != nil || !allowed {
			return false, err
		}
		hasConcreteScope = true
	}
	return hasConcreteScope, nil
}

func (d *Deps) incidentAlertLinkAllowed(ctx context.Context, link incidents.AlertLink, groupAccess map[string]struct{}) (bool, error) {
	if ruleID := strings.TrimSpace(link.AlertRuleID); ruleID != "" {
		if d.AlertStore == nil {
			return false, fmt.Errorf("alert rule authorization store unavailable")
		}
		rule, ok, err := d.AlertStore.GetAlertRule(ruleID)
		if err != nil || !ok {
			return false, err
		}
		return alertRuleAllowed(ctx, rule, groupAccess), nil
	}
	if instanceID := strings.TrimSpace(link.AlertInstanceID); instanceID != "" {
		if d.AlertInstanceStore == nil {
			return false, fmt.Errorf("alert instance authorization store unavailable")
		}
		instance, ok, err := d.AlertInstanceStore.GetAlertInstance(instanceID)
		if err != nil || !ok {
			return false, err
		}
		return d.alertInstanceAllowed(ctx, instance, groupAccess)
	}
	// Fingerprints are not asset identities and cannot be proven safe.
	return false, nil
}

func incidentReferencesAllowed(ctx context.Context, groupID, assetID string, groupAccess map[string]struct{}) bool {
	if !shared.HasAssetRestriction(ctx) {
		return true
	}
	hasConcreteScope := false
	if assetID = strings.TrimSpace(assetID); assetID != "" {
		hasConcreteScope = true
		if !apiv2.AssetCheckContext(ctx, assetID) {
			return false
		}
	}
	if groupID = strings.TrimSpace(groupID); groupID != "" {
		hasConcreteScope = true
		if _, ok := groupAccess[groupID]; !ok {
			return false
		}
	}
	return hasConcreteScope
}

func (d *Deps) requireIncidentAccess(w http.ResponseWriter, r *http.Request, incident incidents.Incident) bool {
	if !shared.HasAssetRestriction(r.Context()) {
		return true
	}
	groupAccess, err := d.accessibleGroupIDs(r.Context())
	if err != nil {
		writeAssetScopeForbidden(w, "unable to prove incident asset scope")
		return false
	}
	allowed, err := d.incidentAllowed(r.Context(), incident, groupAccess)
	if err != nil || !allowed {
		writeAssetScopeForbidden(w, "api key does not have access to every asset referenced by this incident")
		return false
	}
	return true
}

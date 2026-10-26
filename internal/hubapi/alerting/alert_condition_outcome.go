package alerting

import (
	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/assets"
)

// A scope contains positive witnesses, never every candidate of a broad rule.
// No identities is unknown/unowned, not permission to match every incident.
type alertConditionOutcome struct {
	Triggered bool
	Scope     alerts.FiringScope
}

func newAlertConditionOutcome() alertConditionOutcome {
	return alertConditionOutcome{Scope: alerts.FiringScope{Version: 1}}
}

func (o *alertConditionOutcome) witnessAsset(asset assets.Asset) {
	o.Triggered = true
	if asset.ID != "" {
		o.Scope.AssetIDs = append(o.Scope.AssetIDs, asset.ID)
	}
	if asset.GroupID != "" {
		o.Scope.GroupIDs = append(o.Scope.GroupIDs, asset.GroupID)
	}
}

func (o *alertConditionOutcome) includeScope(scope *alerts.FiringScope) {
	if scope == nil {
		return
	}
	normalized, err := alerts.NormalizeFiringScope(*scope)
	if err != nil {
		return
	}
	o.Scope.AssetIDs = append(o.Scope.AssetIDs, normalized.AssetIDs...)
	o.Scope.GroupIDs = append(o.Scope.GroupIDs, normalized.GroupIDs...)
	o.Scope.SyntheticCheckIDs = append(o.Scope.SyntheticCheckIDs, normalized.SyntheticCheckIDs...)
}

func (o alertConditionOutcome) result() (alertConditionOutcome, error) {
	normalized, err := alerts.NormalizeFiringScope(o.Scope)
	o.Scope = normalized
	return o, err
}

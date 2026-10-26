package alerting

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/incidents"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/updates"
)

// Explicit incident assets can match saved alert witnesses. Current group
// membership is separate: it may scope operations, but must not retarget an
// old alert after its asset moves to a different group.
type incidentCorrelationScope struct {
	groupID         string
	assets          map[string]struct{}
	operationAssets map[string]struct{}
	alertLinks      []incidents.AlertLink
}

func correlationID(value string) bool {
	return value != "" && strings.TrimSpace(value) == value && !strings.ContainsRune(value, '\x00')
}

func (d *Deps) incidentCorrelationScope(ctx context.Context, inc incidents.Incident) (incidentCorrelationScope, error) {
	scope := incidentCorrelationScope{groupID: inc.GroupID,
		assets: make(map[string]struct{}), operationAssets: make(map[string]struct{})}
	if !correlationID(inc.ID) || (inc.PrimaryAssetID != "" && !correlationID(inc.PrimaryAssetID)) ||
		(inc.GroupID != "" && !correlationID(inc.GroupID)) {
		return scope, fmt.Errorf("invalid incident scope identity")
	}
	if inc.PrimaryAssetID != "" {
		scope.assets[inc.PrimaryAssetID] = struct{}{}
	}
	if d.DependencyStore != nil {
		owner, ok := d.DependencyStore.(persistence.IncidentAssetScopeReader)
		if !ok {
			return scope, fmt.Errorf("incident asset scope owner unavailable")
		}
		err := owner.ReadIncidentAssetsContext(ctx, inc.ID, func(link incidents.IncidentAsset) error {
			if link.IncidentID != inc.ID || !correlationID(link.AssetID) {
				return fmt.Errorf("invalid incident asset link")
			}
			scope.assets[link.AssetID] = struct{}{}
			return ctx.Err()
		})
		if err != nil {
			return scope, err
		}
	}
	if d.IncidentStore != nil {
		owner, ok := d.IncidentStore.(persistence.IncidentAlertScopeReader)
		if !ok {
			return scope, fmt.Errorf("incident alert scope owner unavailable")
		}
		err := owner.ReadIncidentAlertLinksContext(ctx, inc.ID, func(link incidents.AlertLink) error {
			if link.IncidentID != inc.ID || (link.AlertRuleID == "" && link.AlertInstanceID == "" && link.AlertFingerprint == "") {
				return fmt.Errorf("invalid incident alert link")
			}
			for _, id := range []string{link.AlertRuleID, link.AlertInstanceID, link.AlertFingerprint} {
				if id != "" && !correlationID(id) {
					return fmt.Errorf("invalid alert link identity")
				}
			}
			scope.alertLinks = append(scope.alertLinks, link)
			return ctx.Err()
		})
		if err != nil {
			return scope, err
		}
	}
	for id := range scope.assets {
		scope.operationAssets[id] = struct{}{}
	}
	if inc.GroupID != "" {
		owner, ok := d.AssetStore.(persistence.GroupAssetIDScopeReader)
		if !ok {
			return scope, fmt.Errorf("incident group asset owner unavailable")
		}
		err := owner.ReadGroupAssetIDsContext(ctx, inc.GroupID, func(id string) error {
			if !correlationID(id) {
				return fmt.Errorf("invalid incident group member")
			}
			scope.operationAssets[id] = struct{}{}
			return ctx.Err()
		})
		if err != nil {
			return scope, err
		}
	}
	return scope, ctx.Err()
}

// A deterministic identity restarts only the changed incident's source sweep.
// Separate owner reads are not a cross-table snapshot.
func (s incidentCorrelationScope) pageScope(incidentID string) persistence.IncidentCorrelationScope {
	out := persistence.IncidentCorrelationScope{IncidentID: incidentID, GroupID: s.groupID,
		AlertLinks: append([]incidents.AlertLink(nil), s.alertLinks...)}
	for id := range s.assets {
		out.AssetIDs = append(out.AssetIDs, id)
	}
	for id := range s.operationAssets {
		out.OperationAssetIDs = append(out.OperationAssetIDs, id)
	}
	sort.Strings(out.AssetIDs)
	sort.Strings(out.OperationAssetIDs)
	sort.Slice(out.AlertLinks, func(i, j int) bool { return out.AlertLinks[i].ID < out.AlertLinks[j].ID })
	return out
}

func (s incidentCorrelationScope) matchesOperation(target string) bool {
	_, ok := s.operationAssets[target]
	return correlationID(target) && ok
}

func (s incidentCorrelationScope) matchesUpdate(run updates.Run) bool {
	for _, result := range run.Results {
		if result.Status == updates.StatusFailed && s.matchesOperation(result.Target) {
			return true
		}
	}
	return false
}

func (s incidentCorrelationScope) matchesAlert(inst alerts.AlertInstance) bool {
	if inst.FirstFiredAt == nil || inst.FirstFiredAt.IsZero() {
		return false
	}
	var saved alerts.FiringScope
	if inst.FiringScope != nil {
		var err error
		saved, err = alerts.NormalizeFiringScope(*inst.FiringScope)
		if err != nil {
			return false
		}
	}
	for _, link := range s.alertLinks {
		// Every supplied discriminator must agree. An exact instance link
		// must never degrade to its rule ID when another instance fires.
		if (link.AlertInstanceID == "" || link.AlertInstanceID == inst.ID) &&
			(link.AlertRuleID == "" || link.AlertRuleID == inst.RuleID) &&
			(link.AlertFingerprint == "" || link.AlertFingerprint == inst.Fingerprint) {
			return true
		}
	}
	for _, id := range saved.AssetIDs {
		if _, ok := s.assets[id]; ok {
			return true
		}
	}
	for _, id := range saved.GroupIDs {
		if s.groupID != "" && id == s.groupID {
			return true
		}
	}
	return false
}

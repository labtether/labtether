package alerting

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/persistence"
)

const (
	agentReplayInstanceLimit = 200
	agentReplayResolvedSince = 24 * time.Hour
)

// ReplayAlertsToAgent tells a newly connected agent which alerts are firing
// for its device, and which ended in the last day. The agent keeps alerts in
// memory only, and the Hub otherwise tells it only when an alert starts or
// ends, so after an agent restart or reconnect the Mac and Windows tray lost
// an alert that was still firing (and could keep one that ended meanwhile).
// The agent replaces an alert with the same ID, so a replay never adds a
// duplicate. Best effort: errors are logged and skipped.
func (d *Deps) ReplayAlertsToAgent(ctx context.Context, conn *agentmgr.AgentConn) {
	if conn == nil || d.AlertInstanceStore == nil || d.AlertStore == nil {
		return
	}
	assetID := strings.TrimSpace(conn.AssetID)
	if assetID == "" {
		return
	}
	// First what it missed while away: alerts it showed that have ended.
	for _, data := range d.AgentAlertDevices.takeForLater(assetID) {
		payload, _ := json.Marshal(data)
		if err := conn.SendContext(ctx, agentmgr.Message{Type: agentmgr.MsgAlertNotify, Data: payload}); err != nil {
			d.AgentAlertDevices.keepForLater(assetID, data)
			return
		}
	}
	since := time.Now().UTC().Add(-agentReplayResolvedSince)
	rules := map[string]alerts.Rule{}
	targets := map[string]bool{}
	// Resolved first, so an instance that fired again ends up firing.
	for _, status := range []string{alerts.InstanceStatusResolved, alerts.InstanceStatusAcknowledged, alerts.InstanceStatusFiring} {
		instances, err := d.AlertInstanceStore.ListAlertInstances(persistence.AlertInstanceFilter{Status: status, Limit: agentReplayInstanceLimit})
		if err != nil {
			log.Printf("alert replay: list %s alerts for %s: %v", status, assetID, err)
			continue
		}
		for _, inst := range instances {
			if ctx.Err() != nil {
				return
			}
			if status == alerts.InstanceStatusResolved && (inst.ResolvedAt == nil || inst.ResolvedAt.Before(since)) {
				continue
			}
			rule, ok := d.replayRule(rules, inst.RuleID)
			if !ok {
				continue
			}
			// While an alert fires, the devices breaching now are remembered;
			// otherwise use the devices it first fired for.
			if told, known := d.AgentAlertDevices.toldFor(inst.ID); known && status != alerts.InstanceStatusResolved {
				if _, breaching := told[assetID]; !breaching {
					continue
				}
			} else if !d.instanceTargetsAsset(ctx, inst, rule, assetID, targets) {
				continue
			}
			state, at := "firing", alertFiringSince(inst)
			if status == alerts.InstanceStatusResolved {
				state, at = "resolved", *inst.ResolvedAt
			}
			data, _ := json.Marshal(agentmgr.AlertNotifyData{
				ID: inst.ID, Severity: rule.Severity, Title: rule.Name, Summary: rule.Description,
				State: state, Timestamp: at.UTC().Format(time.RFC3339),
			})
			if err := conn.SendContext(ctx, agentmgr.Message{Type: agentmgr.MsgAlertNotify, Data: data}); err != nil {
				return
			}
		}
	}
}

func (d *Deps) replayRule(cache map[string]alerts.Rule, ruleID string) (alerts.Rule, bool) {
	if rule, ok := cache[ruleID]; ok {
		return rule, rule.ID != ""
	}
	rule, ok, err := d.AlertStore.GetAlertRule(ruleID)
	if err != nil || !ok {
		cache[ruleID] = alerts.Rule{}
		return alerts.Rule{}, false
	}
	cache[ruleID] = rule
	return rule, true
}

// instanceTargetsAsset uses the devices the instance fired for, or the rule's
// targets for an instance recorded without a firing scope.
func (d *Deps) instanceTargetsAsset(ctx context.Context, inst alerts.AlertInstance, rule alerts.Rule, assetID string, cache map[string]bool) bool {
	if inst.FiringScope != nil && len(inst.FiringScope.AssetIDs) > 0 {
		for _, id := range inst.FiringScope.AssetIDs {
			if strings.TrimSpace(id) == assetID {
				return true
			}
		}
		return false
	}
	if hit, ok := cache[rule.ID]; ok {
		return hit
	}
	hit := false
	if targetAssets, err := d.resolveRuleTargetAssetsContext(ctx, rule, nil, nil, true); err == nil {
		for _, target := range targetAssets {
			if strings.TrimSpace(target.ID) == assetID {
				hit = true
				break
			}
		}
	}
	cache[rule.ID] = hit
	return hit
}

// alertFiringSince is when an alert started firing: the time the live
// "firing" message carries. The resync sent the last time it was seen firing,
// so after an agent reconnected the Mac and Windows tray said "since" a few
// seconds ago for an alert that had fired for hours.
func alertFiringSince(inst alerts.AlertInstance) time.Time {
	if inst.FirstFiredAt != nil && !inst.FirstFiredAt.IsZero() {
		return *inst.FirstFiredAt
	}
	return inst.StartedAt
}

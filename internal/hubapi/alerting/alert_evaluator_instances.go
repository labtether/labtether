package alerting

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/persistence"
)

type alertSuppressionPrefetch struct {
	now            time.Time
	activeSilences []alerts.AlertSilence
}

func (d *Deps) newAlertSuppressionPrefetch(ctx context.Context) (*alertSuppressionPrefetch, error) {
	if d.AlertInstanceStore == nil {
		return nil, nil
	}
	store, ok := d.AlertInstanceStore.(persistence.AlertInstanceContextStore)
	if !ok {
		return nil, fmt.Errorf("context-aware alert suppression unavailable")
	}
	silences, err := store.ListAlertSilencesContext(ctx, 500, true)
	if err != nil {
		return nil, err
	}
	// A full page cannot prove that an unseen matching silence is absent.
	if len(silences) >= 500 {
		return nil, fmt.Errorf("alert silence inventory incomplete")
	}
	return &alertSuppressionPrefetch{now: time.Now().UTC(), activeSilences: silences}, nil
}

func (d *Deps) fireOrRefireAlert(rule alerts.Rule, suppressionPrefetch *alertSuppressionPrefetch) {
	d.fireOrRefireAlertAt(context.Background(), rule, suppressionPrefetch, time.Now().UTC())
}

func (d *Deps) fireOrRefireAlertAt(ctx context.Context, rule alerts.Rule, suppressionPrefetch *alertSuppressionPrefetch, at time.Time) {
	d.fireOrRefireAlertWithScopeAt(ctx, rule, suppressionPrefetch, at, alerts.FiringScope{Version: 1})
}

func (d *Deps) fireOrRefireAlertWithScopeAt(ctx context.Context, rule alerts.Rule, suppressionPrefetch *alertSuppressionPrefetch, at time.Time, scope alerts.FiringScope) {
	if d.AlertInstanceStore == nil || ctx.Err() != nil {
		return
	}
	store, capable := d.AlertInstanceStore.(persistence.AlertInstanceContextStore)
	if !capable {
		log.Printf("alert evaluator: context-aware instances unavailable")
		return
	}
	fingerprint := alerts.GenerateFingerprint(rule.ID, rule.Labels)

	createAndDispatchFiringInstance := func() {
		firingStore, ok := d.AlertInstanceStore.(persistence.AlertInstanceFiringStore)
		if !ok {
			log.Printf("alert evaluator: atomic firing scope store unavailable")
			return
		}
		inst, err := store.CreateAlertInstanceContext(ctx, alerts.CreateInstanceRequest{
			RuleID:      rule.ID,
			Fingerprint: fingerprint,
			Severity:    rule.Severity,
			Labels:      rule.Labels,
		})
		if err != nil {
			log.Printf("alert evaluator: failed to create instance: %v", err)
			return
		}
		// Transition to firing.
		if _, err := firingStore.FireAlertInstance(ctx, inst.ID, scope); err != nil {
			log.Printf("alert evaluator: failed to transition instance to firing: %v", err)
			return
		}

		if ctx.Err() != nil {
			return
		}
		d.broadcastEvent("alert.fired", map[string]any{"rule_id": rule.ID, "severity": string(rule.Severity)})
		d.dispatchAlertNotificationsAsync(rule, inst.ID, "firing")

		// Push alert notification to connected agents (best-effort).
		d.pushAlertNotifyToAgents(ctx, rule, inst.ID, "firing")
	}

	existing, ok, err := store.GetActiveInstanceByFingerprintContext(ctx, rule.ID, fingerprint)
	if err != nil {
		log.Printf("alert evaluator: failed to check fingerprint: %v", err)
		return
	}

	if ok {
		// Suppressed alerts are represented by pending instances. If suppression
		// has lifted, promote by resolving the placeholder and creating a fresh
		// firing instance so incident timing starts from actual firing time.
		if existing.Status == alerts.InstanceStatusPending {
			if d.isAlertSuppressedContext(ctx, rule, suppressionPrefetch) {
				_ = store.UpdateAlertInstanceLastFiredContext(ctx, existing.ID)
				return
			}
			if !d.alertFingerprintMayFire(ctx, rule, fingerprint, at) {
				return
			}
			if _, err := store.UpdateAlertInstanceStatusContext(ctx, existing.ID, alerts.InstanceStatusResolved); err != nil {
				log.Printf("alert evaluator: failed to resolve pending suppressed instance %s: %v", existing.ID, err)
				return
			}
			createAndDispatchFiringInstance()
			return
		}

		// Refire: update last_fired_at.
		if err := store.UpdateAlertInstanceLastFiredContext(ctx, existing.ID); err != nil {
			return
		}
		d.maybeAutoCreateIncidentContext(ctx, rule, existing)
		return
	}

	// Check suppression: active silences
	if d.isAlertSuppressedContext(ctx, rule, suppressionPrefetch) {
		_, _ = store.CreateAlertInstanceContext(ctx, alerts.CreateInstanceRequest{
			RuleID:      rule.ID,
			Fingerprint: fingerprint,
			Severity:    rule.Severity,
			Labels:      rule.Labels,
		})
		return
	}

	if d.alertFingerprintMayFire(ctx, rule, fingerprint, at) {
		createAndDispatchFiringInstance()
	}
}

func (d *Deps) isAlertSuppressedContext(ctx context.Context, rule alerts.Rule, prefetch *alertSuppressionPrefetch) bool {
	if ctx.Err() != nil {
		return true
	}
	if d.isAlertMaintenanceSuppressedContext(ctx, rule) {
		return true
	}
	var silences []alerts.AlertSilence
	if prefetch != nil {
		silences = prefetch.activeSilences
	} else {
		store, ok := d.AlertInstanceStore.(persistence.AlertInstanceContextStore)
		if !ok {
			return true
		}
		loaded, err := store.ListAlertSilencesContext(ctx, 500, true)
		if err != nil || len(loaded) >= 500 {
			return true
		}
		silences = loaded
	}
	for _, silence := range silences {
		if matchesSilence(rule, silence) {
			return true
		}
	}

	return false
}

func matchesSilence(rule alerts.Rule, silence alerts.AlertSilence) bool {
	if len(silence.Matchers) == 0 {
		return false
	}
	for key, pattern := range silence.Matchers {
		labelVal, ok := rule.Labels[key]
		if !ok || labelVal != pattern {
			return false
		}
	}
	return true
}

func (d *Deps) resolveStaleInstances(rule alerts.Rule, suppressionPrefetch *alertSuppressionPrefetch) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	d.resolveStaleInstancesContext(ctx, rule, suppressionPrefetch)
}

// resolveStaleInstancesContext resolves up to 100 open instances per status
// and reports how many it resolved.
func (d *Deps) resolveStaleInstancesContext(ctx context.Context, rule alerts.Rule, suppressionPrefetch *alertSuppressionPrefetch) int {
	if d.AlertInstanceStore == nil {
		return 0
	}
	store, ok := d.AlertInstanceStore.(persistence.AlertInstanceContextStore)
	if !ok || ctx.Err() != nil {
		return 0
	}
	resolved := 0
	suppressed := d.isAlertSuppressedContext(ctx, rule, suppressionPrefetch)
	// Keep each status batch bounded. Resolved history must not consume the
	// active-instance limit, and acknowledged/pending rows must also clear.
	for _, status := range []string{alerts.InstanceStatusPending, alerts.InstanceStatusFiring, alerts.InstanceStatusAcknowledged} {
		instances, err := store.ListAlertInstancesContext(ctx, persistence.AlertInstanceFilter{
			RuleID: rule.ID,
			Status: status,
			Limit:  100,
		})
		if err != nil {
			log.Printf("alert evaluator: failed to list %s instances for rule %s: %v", status, rule.ID, err)
			continue
		}
		for _, inst := range instances {
			if ctx.Err() != nil {
				return resolved
			}
			if _, err := store.UpdateAlertInstanceStatusContext(ctx, inst.ID, alerts.InstanceStatusResolved); err != nil {
				log.Printf("alert evaluator: failed to resolve instance %s: %v", inst.ID, err)
				continue
			}
			resolved++
			// A suppressed placeholder has never fired, so it has no recovery
			// notification. Acknowledgement does not suppress genuine recovery.
			if ctx.Err() != nil {
				return resolved
			}
			if suppressed || inst.Status == alerts.InstanceStatusPending {
				continue
			}
			d.broadcastEvent("alert.resolved", map[string]any{"rule_id": rule.ID, "instance_id": inst.ID})
			d.dispatchAlertNotificationsAsync(rule, inst.ID, "resolved")

			// Push resolved notification to connected agents (best-effort).
			d.pushAlertNotifyToAgents(ctx, rule, inst.ID, "resolved")
		}
	}
	return resolved
}

// pushAlertNotifyToAgents sends an alert.notify message to connected agents
// for each asset targeted by the rule. This is best-effort; send errors are ignored.
func (d *Deps) pushAlertNotifyToAgents(ctx context.Context, rule alerts.Rule, instanceID string, state string) {
	if d.AgentMgr == nil {
		return
	}

	targetAssets, err := d.resolveRuleTargetAssetsContext(ctx, rule, nil, nil, true)
	if err != nil {
		log.Printf("alert evaluator: failed to resolve agent notification targets: %v", err)
		return
	}

	for _, targetAsset := range targetAssets {
		if ctx.Err() != nil {
			return
		}
		assetID := strings.TrimSpace(targetAsset.ID)
		if !d.AgentMgr.IsConnected(assetID) {
			continue
		}
		conn, ok := d.AgentMgr.Get(assetID)
		if !ok {
			continue
		}
		notifyData, _ := json.Marshal(agentmgr.AlertNotifyData{
			ID:        instanceID,
			Severity:  rule.Severity,
			Title:     rule.Name,
			Summary:   rule.Description,
			State:     state,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		})
		_ = conn.SendContext(ctx, agentmgr.Message{Type: agentmgr.MsgAlertNotify, Data: notifyData})
	}
}

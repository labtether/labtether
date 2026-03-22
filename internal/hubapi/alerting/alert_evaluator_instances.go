package alerting

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/incidents"
	"github.com/labtether/labtether/internal/persistence"
)

type alertSuppressionPrefetch struct {
	now            time.Time
	activeSilences []alerts.AlertSilence
}

type autoIncidentAlertLinkChecker interface {
	HasAutoIncidentForAlertInstance(alertInstanceID string) (bool, error)
}

func (d *Deps) newAlertSuppressionPrefetch() *alertSuppressionPrefetch {
	if d.AlertInstanceStore == nil {
		return nil
	}

	prefetch := &alertSuppressionPrefetch{
		now:            time.Now().UTC(),
		activeSilences: nil,
	}

	if d.AlertInstanceStore != nil {
		silences, err := d.AlertInstanceStore.ListAlertSilences(500, true)
		if err != nil {
			log.Printf("alert evaluator: failed to prefetch alert silences: %v", err)
		} else {
			prefetch.activeSilences = silences
		}
	}

	return prefetch
}

func (d *Deps) fireOrRefireAlert(rule alerts.Rule, suppressionPrefetch *alertSuppressionPrefetch) {
	if d.AlertInstanceStore == nil {
		return
	}
	fingerprint := alerts.GenerateFingerprint(rule.ID, rule.Labels)

	createAndDispatchFiringInstance := func() {
		inst, err := d.AlertInstanceStore.CreateAlertInstance(alerts.CreateInstanceRequest{
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
		if _, err := d.AlertInstanceStore.UpdateAlertInstanceStatus(inst.ID, alerts.InstanceStatusFiring); err != nil {
			log.Printf("alert evaluator: failed to transition instance to firing: %v", err)
			return
		}

		d.broadcastEvent("alert.fired", map[string]any{"rule_id": rule.ID, "severity": string(rule.Severity)})
		d.dispatchAlertNotificationsAsync(rule, inst.ID, "firing")

		// Push alert notification to connected agents (best-effort).
		d.pushAlertNotifyToAgents(rule, inst.ID, "firing")
	}

	existing, ok, err := d.AlertInstanceStore.GetActiveInstanceByFingerprint(rule.ID, fingerprint)
	if err != nil {
		log.Printf("alert evaluator: failed to check fingerprint: %v", err)
		return
	}

	if ok {
		// Suppressed alerts are represented by pending instances. If suppression
		// has lifted, promote by resolving the placeholder and creating a fresh
		// firing instance so incident timing starts from actual firing time.
		if existing.Status == alerts.InstanceStatusPending {
			if d.isAlertSuppressed(rule, suppressionPrefetch) {
				_ = d.AlertInstanceStore.UpdateAlertInstanceLastFired(existing.ID)
				return
			}
			if _, err := d.AlertInstanceStore.UpdateAlertInstanceStatus(existing.ID, alerts.InstanceStatusResolved); err != nil {
				log.Printf("alert evaluator: failed to resolve pending suppressed instance %s: %v", existing.ID, err)
				return
			}
			createAndDispatchFiringInstance()
			return
		}

		// Refire: update last_fired_at.
		_ = d.AlertInstanceStore.UpdateAlertInstanceLastFired(existing.ID)
		d.MaybeAutoCreateIncident(rule, existing)
		return
	}

	// Check suppression: active silences
	if d.isAlertSuppressed(rule, suppressionPrefetch) {
		_, _ = d.AlertInstanceStore.CreateAlertInstance(alerts.CreateInstanceRequest{
			RuleID:      rule.ID,
			Fingerprint: fingerprint,
			Severity:    rule.Severity,
			Labels:      rule.Labels,
		})
		return
	}

	createAndDispatchFiringInstance()
}

func (d *Deps) MaybeAutoCreateIncident(rule alerts.Rule, instance alerts.AlertInstance) {
	if d.IncidentStore == nil {
		return
	}

	// Only auto-create for critical severity
	if rule.Severity != alerts.SeverityCritical {
		return
	}

	// Only if firing for > 5 minutes
	if time.Since(instance.StartedAt) < 5*time.Minute {
		return
	}

	if checker, ok := d.IncidentStore.(autoIncidentAlertLinkChecker); ok {
		exists, err := checker.HasAutoIncidentForAlertInstance(instance.ID)
		if err != nil {
			log.Printf("alert evaluator: failed to check auto incident link existence: %v", err)
			return
		}
		if exists {
			return
		}
	} else {
		// Fallback for older store implementations.
		autoIncidents, err := d.IncidentStore.ListIncidents(persistence.IncidentFilter{
			Limit:  100,
			Source: incidents.SourceAlertAuto,
		})
		if err != nil {
			log.Printf("alert evaluator: failed to list auto incidents: %v", err)
			return
		}
		for _, inc := range autoIncidents {
			links, err := d.IncidentStore.ListIncidentAlertLinks(inc.ID, 50)
			if err != nil {
				continue
			}
			for _, link := range links {
				if link.AlertInstanceID == instance.ID {
					return // Already linked
				}
			}
		}
	}

	// Create incident
	inc, err := d.IncidentStore.CreateIncident(incidents.CreateIncidentRequest{
		Title:    fmt.Sprintf("Auto: %s", rule.Name),
		Severity: incidents.SeverityCritical,
		Source:   incidents.SourceAlertAuto,
		Summary:  fmt.Sprintf("Auto-created from critical alert rule %q firing for >5 minutes", rule.Name),
	})
	if err != nil {
		log.Printf("alert evaluator: failed to auto-create incident: %v", err)
		return
	}

	// Link alert to incident
	if _, err := d.IncidentStore.LinkIncidentAlert(inc.ID, incidents.LinkAlertRequest{
		AlertInstanceID: instance.ID,
		LinkType:        incidents.LinkTypeTrigger,
	}); err != nil {
		log.Printf("alert evaluator: failed to link alert to incident: %v", err)
	}

	log.Printf("alert evaluator: auto-created incident %s from critical alert %s", inc.ID, rule.Name)
}

func (d *Deps) isAlertSuppressed(rule alerts.Rule, prefetch *alertSuppressionPrefetch) bool {
	var silences []alerts.AlertSilence
	if prefetch != nil {
		silences = prefetch.activeSilences
	} else {
		loaded, err := d.AlertInstanceStore.ListAlertSilences(100, true)
		if err != nil {
			return false
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

func (d *Deps) resolveStaleInstances(rule alerts.Rule) {
	if d.AlertInstanceStore == nil {
		return
	}
	instances, err := d.AlertInstanceStore.ListAlertInstances(persistence.AlertInstanceFilter{
		RuleID: rule.ID,
		Status: alerts.InstanceStatusFiring,
		Limit:  100,
	})
	if err != nil {
		return
	}
	for _, inst := range instances {
		_, _ = d.AlertInstanceStore.UpdateAlertInstanceStatus(inst.ID, alerts.InstanceStatusResolved)
		d.broadcastEvent("alert.resolved", map[string]any{"rule_id": rule.ID, "instance_id": inst.ID})
		d.dispatchAlertNotificationsAsync(rule, inst.ID, "resolved")

		// Push resolved notification to connected agents (best-effort)
		d.pushAlertNotifyToAgents(rule, inst.ID, "resolved")
	}
}

// pushAlertNotifyToAgents sends an alert.notify message to connected agents
// for each asset targeted by the rule. This is best-effort; send errors are ignored.
func (d *Deps) pushAlertNotifyToAgents(rule alerts.Rule, instanceID string, state string) {
	if d.AgentMgr == nil {
		return
	}

	targetAssets, err := d.ResolveRuleTargetAssets(rule, nil, true)
	if err != nil {
		log.Printf("alert evaluator: failed to resolve agent notification targets: %v", err)
		return
	}

	for _, targetAsset := range targetAssets {
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
		_ = conn.Send(agentmgr.Message{Type: agentmgr.MsgAlertNotify, Data: notifyData})
	}
}

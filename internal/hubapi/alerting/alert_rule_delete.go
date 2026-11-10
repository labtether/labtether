package alerting

import (
	"context"
	"log"
	"time"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/persistence"
)

// agentAlertIDsForRule lists the alerts of a rule that agents were told are
// firing (acknowledged ones too: nothing told the agent otherwise).
func (d *Deps) agentAlertIDsForRule(ruleID string) []string {
	if d.AlertInstanceStore == nil || d.AgentMgr == nil {
		return nil
	}
	var ids []string
	for _, status := range []string{alerts.InstanceStatusFiring, alerts.InstanceStatusAcknowledged} {
		instances, err := d.AlertInstanceStore.ListAlertInstances(persistence.AlertInstanceFilter{RuleID: ruleID, Status: status, Limit: agentReplayInstanceLimit})
		if err != nil {
			log.Printf("alert rule delete: list %s alerts of %s: %v", status, ruleID, err)
			continue
		}
		for _, inst := range instances {
			ids = append(ids, inst.ID)
		}
	}
	return ids
}

// tellAgentsAlertsEnded sends "resolved" for alerts that ended without the
// evaluator. Deleting a rule deletes its alerts, and the agent (the Mac and
// Windows tray) kept showing them as firing until it restarted. The devices
// told "firing" get it.
func (d *Deps) tellAgentsAlertsEnded(rule alerts.Rule, ids []string) {
	if len(ids) == 0 || d.AgentMgr == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, id := range ids {
		d.pushAlertResolvedToAgents(ctx, rule, id)
	}
}

// tellAgentsAlertResolved sends "resolved" for an alert someone resolved by
// hand. The evaluator did not end it, so nothing told the agents, and the
// tray kept showing it as firing. If the rule is still breached the
// evaluator opens a new alert, which the breaching devices are told about.
func (d *Deps) tellAgentsAlertResolved(inst alerts.AlertInstance) {
	if d.AgentMgr == nil || d.AlertStore == nil {
		return
	}
	rule, ok, err := d.AlertStore.GetAlertRule(inst.RuleID)
	if err != nil || !ok {
		return
	}
	d.tellAgentsAlertsEnded(rule, []string{inst.ID})
}

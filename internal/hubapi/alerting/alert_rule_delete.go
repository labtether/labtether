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
// Windows tray) kept showing them as firing until it restarted. Same targets
// as the "firing" message the evaluator sent.
func (d *Deps) tellAgentsAlertsEnded(rule alerts.Rule, ids []string) {
	if len(ids) == 0 || d.AgentMgr == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, id := range ids {
		d.pushAlertNotifyToAgents(ctx, rule, id, "resolved")
	}
}

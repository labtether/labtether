package alerting

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/alerts"
)

// sendAlertState sends one alert.notify to each device's agent. It returns
// the message and the devices it did not reach (not connected, or the send
// failed). Best effort: nothing is retried here.
func (d *Deps) sendAlertState(ctx context.Context, rule alerts.Rule, instanceID, state string, deviceIDs []string) (agentmgr.AlertNotifyData, []string) {
	data := agentmgr.AlertNotifyData{
		ID:        instanceID,
		Severity:  rule.Severity,
		Title:     rule.Name,
		Summary:   rule.Description,
		State:     state,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	if d.AgentMgr == nil {
		return data, deviceIDs
	}
	payload, _ := json.Marshal(data)
	var missed []string
	for _, raw := range deviceIDs {
		deviceID := strings.TrimSpace(raw)
		if deviceID == "" {
			continue
		}
		conn, ok := d.AgentMgr.Get(deviceID)
		if !ok || ctx.Err() != nil || conn.SendContext(ctx, agentmgr.Message{Type: agentmgr.MsgAlertNotify, Data: payload}) != nil {
			missed = append(missed, deviceID)
		}
	}
	return data, missed
}

// ruleDeviceIDs lists every device the rule watches.
func (d *Deps) ruleDeviceIDs(ctx context.Context, rule alerts.Rule) []string {
	targets, err := d.resolveRuleTargetAssetsContext(ctx, rule, nil, nil, true)
	if err != nil {
		log.Printf("alert evaluator: failed to resolve agent notification targets: %v", err)
		return nil
	}
	ids := make([]string, 0, len(targets))
	for _, target := range targets {
		if id := strings.TrimSpace(target.ID); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

// pushAlertFiringToAgents tells agents that a new alert is firing. Only the
// devices it fired for are told: a rule that watches ten devices used to
// light up all ten trays when one breached. A rule whose alert names no
// device (a check, a composite) still tells every device it watches.
func (d *Deps) pushAlertFiringToAgents(ctx context.Context, rule alerts.Rule, instanceID string, scope alerts.FiringScope) {
	if d.AgentMgr == nil {
		return
	}
	devices := scope.AssetIDs
	if len(devices) == 0 {
		devices = d.ruleDeviceIDs(ctx, rule)
	}
	d.sendAlertState(ctx, rule, instanceID, "firing", devices)
	d.AgentAlertDevices.setTold(instanceID, devices)
}

// syncAlertFiringDevices runs each time a firing alert's rule is still
// breached. A device that starts breaching is told "firing"; one that
// stopped while others keep the alert open is told "resolved". On the first
// pass after the Hub started nothing is remembered: the breaching devices
// are told again (agents replace an alert by ID) and every other device the
// rule watches is told "resolved" once, which also clears alerts an older
// Hub sent to all of them.
func (d *Deps) syncAlertFiringDevices(ctx context.Context, rule alerts.Rule, inst alerts.AlertInstance, scope alerts.FiringScope) {
	if d.AgentMgr == nil || d.AgentAlertDevices == nil || len(scope.AssetIDs) == 0 {
		return
	}
	breaching := make(map[string]struct{}, len(scope.AssetIDs))
	for _, id := range scope.AssetIDs {
		breaching[id] = struct{}{}
	}
	told, known := d.AgentAlertDevices.toldFor(inst.ID)
	if !known {
		for _, id := range d.ruleDeviceIDs(ctx, rule) {
			told[id] = struct{}{}
		}
		if inst.FiringScope != nil {
			for _, id := range inst.FiringScope.AssetIDs {
				told[id] = struct{}{}
			}
		}
	}
	var started, stopped []string
	for id := range breaching {
		if _, was := told[id]; !was || !known {
			started = append(started, id)
		}
	}
	for id := range told {
		if _, still := breaching[id]; !still {
			stopped = append(stopped, id)
		}
	}
	d.sendAlertState(ctx, rule, inst.ID, "firing", started)
	data, missed := d.sendAlertState(ctx, rule, inst.ID, "resolved", stopped)
	if known {
		for _, id := range missed {
			d.AgentAlertDevices.keepForLater(id, data)
		}
	}
	d.AgentAlertDevices.setTold(inst.ID, scope.AssetIDs)
}

// pushAlertResolvedToAgents tells agents that an alert ended. The devices
// told it was firing get the message; one that is away gets it when it
// connects again. When nothing is remembered (the Hub restarted and has not
// evaluated the rule since) every device the rule watches is told.
func (d *Deps) pushAlertResolvedToAgents(ctx context.Context, rule alerts.Rule, instanceID string) {
	if d.AgentMgr == nil {
		return
	}
	told, known := d.AgentAlertDevices.toldFor(instanceID)
	devices := make([]string, 0, len(told))
	for id := range told {
		devices = append(devices, id)
	}
	if !known {
		devices = d.ruleDeviceIDs(ctx, rule)
	}
	data, missed := d.sendAlertState(ctx, rule, instanceID, "resolved", devices)
	if known {
		for _, id := range missed {
			d.AgentAlertDevices.keepForLater(id, data)
		}
	}
	d.AgentAlertDevices.forget(instanceID)
}

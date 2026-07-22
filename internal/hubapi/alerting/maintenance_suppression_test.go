package alerting

import (
	"testing"
	"time"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/hubapi/groupfeatures"
	"github.com/labtether/labtether/internal/persistence"
)

func suppressingGuardrails(groupID string, _ time.Time) (groupfeatures.GroupMaintenanceGuardrails, error) {
	return groupfeatures.GroupMaintenanceGuardrails{
		GroupID:        groupID,
		SuppressAlerts: groupID == "group-1",
	}, nil
}

func TestMaintenanceSuppressesTargetedAlertAsPending(t *testing.T) {
	instanceStore := persistence.NewMemoryAlertInstanceStore()
	deps := &Deps{
		AlertInstanceStore: instanceStore,
		EvaluateGuardrails: suppressingGuardrails,
	}
	rule := alerts.Rule{
		ID:       "rule-1",
		Name:     "Node offline",
		Severity: alerts.SeverityHigh,
		Targets:  []alerts.RuleTarget{{GroupID: "group-1"}},
	}

	deps.fireOrRefireAlert(rule, nil)
	instances, err := instanceStore.ListAlertInstances(persistence.AlertInstanceFilter{RuleID: rule.ID, Limit: 10})
	if err != nil {
		t.Fatalf("list instances: %v", err)
	}
	if len(instances) != 1 || instances[0].Status != alerts.InstanceStatusPending {
		t.Fatalf("suppressed instances = %+v, want one pending placeholder", instances)
	}
}

func TestMaintenanceSuppressionLiftingPromotesFreshFiringInstance(t *testing.T) {
	instanceStore := persistence.NewMemoryAlertInstanceStore()
	suppressed := true
	deps := &Deps{
		AlertInstanceStore: instanceStore,
		EvaluateGuardrails: func(groupID string, _ time.Time) (groupfeatures.GroupMaintenanceGuardrails, error) {
			return groupfeatures.GroupMaintenanceGuardrails{GroupID: groupID, SuppressAlerts: suppressed}, nil
		},
	}
	rule := alerts.Rule{
		ID:       "rule-1",
		Name:     "Node offline",
		Severity: alerts.SeverityHigh,
		Targets:  []alerts.RuleTarget{{GroupID: "group-1"}},
	}

	deps.fireOrRefireAlert(rule, nil)
	suppressed = false
	deps.fireOrRefireAlert(rule, nil)

	instances, err := instanceStore.ListAlertInstances(persistence.AlertInstanceFilter{RuleID: rule.ID, Limit: 10})
	if err != nil {
		t.Fatalf("list instances: %v", err)
	}
	if len(instances) != 2 {
		t.Fatalf("instances = %+v, want resolved placeholder and fresh firing instance", instances)
	}
	statuses := map[string]int{}
	for _, instance := range instances {
		statuses[instance.Status]++
	}
	if statuses[alerts.InstanceStatusResolved] != 1 || statuses[alerts.InstanceStatusFiring] != 1 {
		t.Fatalf("status counts = %+v, want one resolved and one firing", statuses)
	}
}

func TestMaintenanceSuppressesResolvedBroadcastDelivery(t *testing.T) {
	instanceStore := persistence.NewMemoryAlertInstanceStore()
	rule := alerts.Rule{
		ID:       "rule-1",
		Name:     "Node offline",
		Severity: alerts.SeverityHigh,
		Targets:  []alerts.RuleTarget{{GroupID: "group-1"}},
	}
	instance, err := instanceStore.CreateAlertInstance(alerts.CreateInstanceRequest{
		RuleID:      rule.ID,
		Fingerprint: alerts.GenerateFingerprint(rule.ID, rule.Labels),
		Severity:    rule.Severity,
	})
	if err != nil {
		t.Fatalf("create instance: %v", err)
	}
	if _, err := instanceStore.UpdateAlertInstanceStatus(instance.ID, alerts.InstanceStatusFiring); err != nil {
		t.Fatalf("fire instance: %v", err)
	}
	broadcasts := 0
	deps := &Deps{
		AlertInstanceStore: instanceStore,
		EvaluateGuardrails: suppressingGuardrails,
		Broadcast: func(string, map[string]any) {
			broadcasts++
		},
	}

	deps.resolveStaleInstances(rule, nil)
	resolved, ok, err := instanceStore.GetAlertInstance(instance.ID)
	if err != nil || !ok {
		t.Fatalf("load resolved instance: ok=%v err=%v", ok, err)
	}
	if resolved.Status != alerts.InstanceStatusResolved {
		t.Fatalf("status = %q, want resolved", resolved.Status)
	}
	if broadcasts != 0 {
		t.Fatalf("resolved broadcasts = %d, want none during suppression", broadcasts)
	}
}

func TestNotificationPayloadGroupIDsAreBoundedToStringValues(t *testing.T) {
	groupIDs := notificationPayloadGroupIDs(map[string]any{
		"group_ids": []any{" group-1 ", 42, "", "group-2", "group-1"},
	})
	if len(groupIDs) != 2 {
		t.Fatalf("group ids = %+v, want two unique values", groupIDs)
	}
	if _, ok := groupIDs["group-1"]; !ok {
		t.Fatal("missing group-1")
	}
	if _, ok := groupIDs["group-2"]; !ok {
		t.Fatal("missing group-2")
	}
}

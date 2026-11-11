package alerting

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/alerts"
)

func runningSilence(t *testing.T, d *Deps, matchers map[string]string) {
	t.Helper()
	if _, err := d.AlertInstanceStore.CreateAlertSilence(alerts.CreateSilenceRequest{
		Matchers: matchers, StartsAt: time.Now().Add(-time.Minute), EndsAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
}

func fireForDevices(t *testing.T, d *Deps, rule alerts.Rule, devices ...string) alerts.AlertInstance {
	t.Helper()
	d.fireOrRefireAlertWithScopeAt(context.Background(), rule, nil, time.Now().UTC(), alerts.FiringScope{Version: 1, AssetIDs: devices})
	d.WaitForNotificationDispatches()
	inst, ok, err := d.AlertInstanceStore.GetActiveInstanceByFingerprint(rule.ID, alerts.GenerateFingerprint(rule.ID, rule.Labels))
	if err != nil || !ok {
		t.Fatalf("no alert instance (err %v)", err)
	}
	return inst
}

// Silences matched rule labels only, so a rule without labels could not be
// silenced from an alert, and a silence always muted the rule on every
// device. A silence may name the rule and a device: it mutes that device only.
func TestDeviceSilenceMutesOnlyThatDevice(t *testing.T) {
	d, _ := firingScopeDeps(t)
	rule := scopeRule(t, d, alerts.RuleKindHeartbeatStale, map[string]any{"max_stale_seconds": 60.0}, false)
	runningSilence(t, d, map[string]string{"rule_id": rule.ID, "asset_id": "scope-a"})
	inst := fireForDevices(t, d, rule, "scope-a", "scope-b")
	if inst.Status != alerts.InstanceStatusFiring || inst.FiringScope == nil || !reflect.DeepEqual(inst.FiringScope.AssetIDs, []string{"scope-b"}) {
		t.Fatalf("alert = %s %+v, want firing for scope-b only", inst.Status, inst.FiringScope)
	}
	if !d.isAlertSilencedAtDelivery(rule, []string{"scope-a"}) || d.isAlertSilencedAtDelivery(rule, []string{"scope-a", "scope-b"}) {
		t.Fatal("delivery must stop for an alert on the silenced device only, not one that also covers scope-b")
	}

	both := scopeRule(t, d, alerts.RuleKindHeartbeatStale, map[string]any{"max_stale_seconds": 60.0}, false)
	runningSilence(t, d, map[string]string{"rule_id": both.ID, "asset_id": "scope-a"})
	runningSilence(t, d, map[string]string{"rule_id": both.ID, "asset_id": "scope-b"})
	if inst := fireForDevices(t, d, both, "scope-a", "scope-b"); inst.Status != alerts.InstanceStatusPending {
		t.Fatalf("with every device silenced the alert is %s, want it waiting (pending)", inst.Status)
	}
}

// Old silences that match rule labels keep working, and a silence on the rule
// alone mutes it everywhere.
func TestLabelAndRuleSilencesStillMuteTheRule(t *testing.T) {
	d, _ := firingScopeDeps(t)
	labelled, err := d.AlertStore.CreateAlertRule(alerts.CreateRuleRequest{Name: "labelled", Kind: alerts.RuleKindHeartbeatStale, Severity: "high",
		TargetScope: alerts.TargetScopeAsset, Targets: []alerts.RuleTargetInput{{AssetID: "scope-a"}}, Labels: map[string]string{"team": "ops"},
		Condition: map[string]any{"max_stale_seconds": 60.0}})
	if err != nil {
		t.Fatal(err)
	}
	runningSilence(t, d, map[string]string{"team": "ops"})
	if inst := fireForDevices(t, d, labelled, "scope-a"); inst.Status != alerts.InstanceStatusPending {
		t.Fatalf("label silence: alert is %s, want pending", inst.Status)
	}
	whole := scopeRule(t, d, alerts.RuleKindHeartbeatStale, map[string]any{"max_stale_seconds": 60.0}, false)
	runningSilence(t, d, map[string]string{"rule_id": whole.ID})
	if inst := fireForDevices(t, d, whole, "scope-a", "scope-b"); inst.Status != alerts.InstanceStatusPending {
		t.Fatalf("rule silence: alert is %s, want pending", inst.Status)
	}
}

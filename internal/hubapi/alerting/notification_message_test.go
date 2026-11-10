package alerting

import (
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/persistence"
)

// What a person reads in an email, Slack, ntfy, Gotify or a push says which
// device the alert is for, and carries no Hub-made IDs.
func TestAlertNotificationNamesTheDeviceAndHidesHubIDs(t *testing.T) {
	d := newTestAlertingDeps(t)
	store := persistence.NewMemoryAssetStore()
	d.AssetStore = store
	if _, err := store.UpsertAssetHeartbeat(assets.HeartbeatRequest{AssetID: "nas01-agent", Name: "nas01", Type: "host", Source: "agent", Status: "online"}); err != nil {
		t.Fatal(err)
	}
	rule := alerts.Rule{ID: "rule_1000000000000000001_1", Name: "Disk nearly full", Severity: "high", Kind: "metric_threshold"}
	inst, err := d.AlertInstanceStore.CreateAlertInstance(alerts.CreateInstanceRequest{
		RuleID: rule.ID, Fingerprint: "nas01-disk", Severity: "high", Labels: map[string]string{"asset_id": "nas01-agent"},
	})
	if err != nil {
		t.Fatal(err)
	}

	payload := buildAlertNotificationPayload(rule, inst.ID, "firing", nil, d.alertNotificationDevices(inst.ID))
	text, _ := payload["text"].(string)
	if !strings.HasPrefix(text, "Disk nearly full is firing on nas01.\nSeverity: high\nTime: ") {
		t.Fatalf("text = %q, want the rule, state and device first", text)
	}
	for _, internal := range []string{rule.ID, inst.ID, "Instance:", "No description provided"} {
		if strings.Contains(text, internal) {
			t.Fatalf("text %q contains %q", text, internal)
		}
	}
	if got, want := payload["title"], "[HIGH] Disk nearly full on nas01 (firing)"; got != want {
		t.Fatalf("title = %q, want %q", got, want)
	}
	email, err := buildEmailNotificationPayload(map[string]any{"to": "ops@example.com"}, payload)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := email["subject"], "[HIGH] Disk nearly full on nas01 (firing)"; got != want {
		t.Fatalf("email subject = %q, want %q", got, want)
	}
	if got := alertDeviceSummary([]string{"a", "b", "c", "d", "e"}); got != "a, b and 3 more" {
		t.Fatalf("summary of five devices = %q", got)
	}
}

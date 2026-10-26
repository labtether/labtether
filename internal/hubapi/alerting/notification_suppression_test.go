package alerting

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/hubapi/groupfeatures"
	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/notifications"
	"github.com/labtether/labtether/internal/persistence"
)

type suppressionNotificationStore struct {
	*apnsRetryNotificationStore
	channelIDs []string
}

func (s *suppressionNotificationStore) ListAlertRoutes(int) ([]notifications.Route, error) {
	return []notifications.Route{{ID: "suppression-route", Enabled: true, ChannelIDs: s.channelIDs}}, nil
}

type unavailableSilenceStore struct {
	persistence.AlertInstanceStore
	fullPage bool
}

func (s *unavailableSilenceStore) ListAlertSilences(int, bool) ([]alerts.AlertSilence, error) {
	if s.fullPage {
		return make([]alerts.AlertSilence, 500), nil
	}
	return nil, errors.New("silence store unavailable")
}

func newSuppressionDelivery(t *testing.T, channelIDs ...string) (*Deps, *suppressionNotificationStore, alerts.Rule, string) {
	t.Helper()
	d := newTestAlertingDeps(t)
	d.NotificationSecrets = testutil.TestSecretsManager(t)
	history := &suppressionNotificationStore{
		apnsRetryNotificationStore: &apnsRetryNotificationStore{newNotificationSecurityStore()},
		channelIDs:                 channelIDs,
	}
	d.NotificationStore = history
	for _, id := range channelIDs {
		history.seed(notifications.Channel{ID: id, Type: notifications.ChannelTypeWebhook, Enabled: true,
			Config: map[string]any{"url": "https://example.invalid/unused"}})
	}
	rule, err := d.AlertStore.CreateAlertRule(alerts.CreateRuleRequest{
		Name: "Delivery suppression", Kind: alerts.RuleKindMetricThreshold, Severity: alerts.SeverityHigh,
		Labels: map[string]string{"team": "operations"}, Targets: []alerts.RuleTargetInput{{GroupID: "maintenance-group"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	instance, err := d.AlertInstanceStore.CreateAlertInstance(alerts.CreateInstanceRequest{
		RuleID: rule.ID, Fingerprint: alerts.GenerateFingerprint(rule.ID, rule.Labels), Severity: rule.Severity,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.AlertInstanceStore.UpdateAlertInstanceStatus(instance.ID, alerts.InstanceStatusFiring); err != nil {
		t.Fatal(err)
	}
	return d, history, rule, instance.ID
}

func silenceForDelivery(t *testing.T, d *Deps, labels map[string]string) string {
	t.Helper()
	silence, err := d.AlertInstanceStore.CreateAlertSilence(alerts.CreateSilenceRequest{
		Matchers: labels, StartsAt: time.Now().Add(-time.Minute), EndsAt: time.Now().Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	return silence.ID
}

func waitDelivery(t *testing.T, d *Deps) {
	t.Helper()
	done := make(chan struct{})
	go func() { d.WaitForNotificationDispatches(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("notification dispatch did not finish")
	}
}

func TestQueuedAlertDispatchChecksCurrentSilence(t *testing.T) {
	for _, condition := range []string{"matching", "unrelated", "read_error", "full_page"} {
		t.Run(condition, func(t *testing.T) {
			d, history, rule, instanceID := newSuppressionDelivery(t, "channel-1")
			adapter := &incidentNotificationAdapterStub{typ: notifications.ChannelTypeWebhook}
			d.NotificationAdapters[notifications.ChannelTypeWebhook] = adapter
			d.NotificationSem = make(chan struct{}, 1)
			d.NotificationSem <- struct{}{}
			var releaseOnce sync.Once
			release := func() { releaseOnce.Do(func() { <-d.NotificationSem }) }
			t.Cleanup(func() { release(); waitDelivery(t, d) })
			d.DispatchAlertNotifications(rule, instanceID, "firing")
			// The dispatch is accepted, but cannot read suppression until released.
			switch condition {
			case "matching":
				silenceForDelivery(t, d, rule.Labels)
			case "unrelated":
				silenceForDelivery(t, d, map[string]string{"team": "another-team"})
			default:
				d.AlertInstanceStore = &unavailableSilenceStore{d.AlertInstanceStore, condition == "full_page"}
			}
			release()
			waitDelivery(t, d)
			want := 0
			if condition == "unrelated" {
				want = 1
			}
			records, err := history.ListNotificationHistory(10, "")
			if err != nil || len(adapter.snapshot()) != want || len(records) != want {
				t.Fatalf("calls=%d records=%d err=%v, want %d", len(adapter.snapshot()), len(records), err, want)
			}
		})
	}
}

func TestAlertRetryDefersSilenceWithoutConsumingAttempt(t *testing.T) {
	for _, condition := range []string{"matching", "read_error", "full_page", "missing_identity"} {
		t.Run(condition, func(t *testing.T) {
			d, history, rule, instanceID := newSuppressionDelivery(t, "channel-1")
			instances := d.AlertInstanceStore
			adapter := &incidentNotificationAdapterStub{typ: notifications.ChannelTypeWebhook}
			d.NotificationAdapters[notifications.ChannelTypeWebhook] = adapter
			// Acknowledgement is triage, not cancellation of an earlier notification.
			if _, err := instances.UpdateAlertInstanceStatus(instanceID, alerts.InstanceStatusAcknowledged); err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(buildAlertNotificationPayload(rule, instanceID, "firing", d.collectRuleGroupIDs(rule)))
			if err != nil {
				t.Fatal(err)
			}
			var payload map[string]any
			if err := json.Unmarshal(encoded, &payload); err != nil {
				t.Fatal(err)
			}
			rec := d.recordNotificationHistoryWithRetry("channel-1", instanceID, "suppression-route", notifications.RecordStatusFailed, "temporary failure", payload)
			if rec.ID == "" {
				t.Fatal("retry record missing")
			}
			due := time.Now().Add(-time.Second)
			history.mu.Lock()
			history.records[0].NextRetryAt = &due
			if condition == "missing_identity" {
				history.records[0].AlertInstanceID = ""
			}
			history.mu.Unlock()
			var silenceID string
			switch condition {
			case "matching":
				silenceID = silenceForDelivery(t, d, rule.Labels)
			case "read_error", "full_page":
				d.AlertInstanceStore = &unavailableSilenceStore{instances, condition == "full_page"}
			}
			d.RetryPendingNotifications(context.Background())
			records, err := history.ListNotificationHistory(10, "")
			if err != nil || len(records) != 1 {
				t.Fatalf("retry history: %+v %v", records, err)
			}
			blocked := records[0]
			if len(adapter.snapshot()) != 0 || blocked.RetryCount != 0 || blocked.Status != notifications.RecordStatusFailed ||
				blocked.NextRetryAt == nil || !blocked.NextRetryAt.Equal(due) {
				t.Fatalf("suppression sent or consumed retry: %+v calls=%d", blocked, len(adapter.snapshot()))
			}
			// Remove only the blocker. The same due record must now deliver once.
			d.AlertInstanceStore = instances
			if silenceID != "" {
				if err := instances.DeleteAlertSilence(silenceID); err != nil {
					t.Fatal(err)
				}
			}
			history.mu.Lock()
			history.records[0].AlertInstanceID = instanceID
			history.mu.Unlock()
			d.RetryPendingNotifications(context.Background())
			d.RetryPendingNotifications(context.Background())
			records, err = history.ListNotificationHistory(10, "")
			if err != nil || len(records) != 1 || len(adapter.snapshot()) != 1 || records[0].RetryCount != 1 ||
				records[0].Status != notifications.RecordStatusSent || records[0].NextRetryAt != nil {
				t.Fatalf("retry did not resume exactly once: %+v calls=%d err=%v", records, len(adapter.snapshot()), err)
			}
		})
	}
}

func TestAlertDispatchRechecksMaintenanceBetweenChannels(t *testing.T) {
	d, history, rule, instanceID := newSuppressionDelivery(t, "channel-1", "channel-2")
	release := make(chan struct{})
	adapter := &incidentNotificationAdapterStub{typ: notifications.ChannelTypeWebhook, started: make(chan struct{}), release: release}
	d.NotificationAdapters[notifications.ChannelTypeWebhook] = adapter
	var maintenance atomic.Bool
	d.EvaluateGuardrails = func(groupID string, _ time.Time) (groupfeatures.GroupMaintenanceGuardrails, error) {
		return groupfeatures.GroupMaintenanceGuardrails{GroupID: groupID, SuppressAlerts: maintenance.Load()}, nil
	}
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(func() { unblock(); waitDelivery(t, d) })
	d.DispatchAlertNotifications(rule, instanceID, "firing")
	select {
	case <-adapter.started:
	case <-time.After(2 * time.Second):
		t.Fatal("first channel never reached adapter")
	}
	maintenance.Store(true)
	unblock()
	waitDelivery(t, d)
	records, err := history.ListNotificationHistory(10, "")
	if err != nil || len(adapter.snapshot()) != 1 || len(records) != 1 || records[0].ChannelID != "channel-1" ||
		records[0].Status != notifications.RecordStatusSent {
		t.Fatalf("second channel escaped maintenance: calls=%d records=%+v err=%v", len(adapter.snapshot()), records, err)
	}
	maintenance.Store(false)
	d.DispatchAlertNotifications(rule, instanceID, "firing")
	waitDelivery(t, d)
	if len(adapter.snapshot()) != 3 {
		t.Fatalf("a new unsuppressed dispatch should reach both channels: %d", len(adapter.snapshot()))
	}
}

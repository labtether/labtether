package alerting

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/notifications"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/telemetry"
)

type recoveryNotificationStore struct {
	*notificationSecurityStore
}

func (s *recoveryNotificationStore) ListAlertRoutes(int) ([]notifications.Route, error) {
	return []notifications.Route{{ID: "recovery-route", Enabled: true, ChannelIDs: []string{"recovery-channel"}}}, nil
}

type recoveryFailureStore struct {
	persistence.AlertInstanceStore
	persistence.AlertInstanceContextStore
	fail bool
}

func (s *recoveryFailureStore) UpdateAlertInstanceStatusContext(ctx context.Context, id, status string) (alerts.AlertInstance, error) {
	if s.fail && status == alerts.InstanceStatusResolved {
		return alerts.AlertInstance{}, errors.New("injected recovery write failure")
	}
	return s.AlertInstanceContextStore.UpdateAlertInstanceStatusContext(ctx, id, status)
}

func (s *recoveryFailureStore) AlertFingerprintTiming(ctx context.Context, ruleID, fingerprint string) (persistence.AlertFingerprintTiming, error) {
	return s.AlertInstanceStore.(persistence.AlertInstanceTimingStore).AlertFingerprintTiming(ctx, ruleID, fingerprint)
}

func (s *recoveryFailureStore) FireAlertInstance(ctx context.Context, id string, scope alerts.FiringScope) (alerts.AlertInstance, error) {
	return s.AlertInstanceStore.(persistence.AlertInstanceFiringStore).FireAlertInstance(ctx, id, scope)
}

func TestAlertRecoveryClearsActiveInstancesTruthfully(t *testing.T) {
	for _, initial := range []string{alerts.InstanceStatusPending, alerts.InstanceStatusFiring, alerts.InstanceStatusAcknowledged, "write_failure"} {
		t.Run(initial, func(t *testing.T) {
			deps := newTestAlertingDeps(t)
			deps.NotificationSecrets = testutil.TestSecretsManager(t)
			ctx := context.Background()
			if _, err := deps.AssetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
				AssetID: "recovery-node", Type: "host", Status: "online", Source: "agent",
			}); err != nil {
				t.Fatal(err)
			}
			if err := deps.TelemetryStore.AppendSamples(ctx, []telemetry.MetricSample{{
				AssetID: "recovery-node", Metric: "cpu_used_percent", Unit: "percent", Value: 90,
				CollectedAt: time.Now().UTC(),
			}}); err != nil {
				t.Fatal(err)
			}
			zero := 0 // This recovery test deliberately requests immediate recurrence.
			rule, err := deps.AlertStore.CreateAlertRule(alerts.CreateRuleRequest{
				CooldownSeconds: &zero, ReopenAfterSeconds: &zero,
				Name: "Recovery", Kind: alerts.RuleKindMetricThreshold, Severity: alerts.SeverityHigh,
				TargetScope: alerts.TargetScopeAsset, Targets: []alerts.RuleTargetInput{{AssetID: "recovery-node"}},
				Labels:    map[string]string{"test": "recovery"},
				Condition: map[string]any{"metric": "cpu_used_percent", "operator": ">", "threshold": float64(80)},
			})
			if err != nil {
				t.Fatal(err)
			}
			notificationStore := &recoveryNotificationStore{newNotificationSecurityStore()}
			if _, err := notificationStore.CreateNotificationChannel(notifications.CreateChannelRequest{
				ID: "recovery-channel", Type: notifications.ChannelTypeWebhook,
				Config: map[string]any{"url": "https://example.invalid/unused"},
			}); err != nil {
				t.Fatal(err)
			}
			adapter := &incidentNotificationAdapterStub{typ: notifications.ChannelTypeWebhook}
			deps.NotificationStore = notificationStore
			deps.NotificationAdapters[notifications.ChannelTypeWebhook] = adapter
			var broadcasts []string
			deps.Broadcast = func(event string, _ map[string]any) { broadcasts = append(broadcasts, event) }
			var silenceID string
			if initial == alerts.InstanceStatusPending {
				silence, err := deps.AlertInstanceStore.CreateAlertSilence(alerts.CreateSilenceRequest{
					Matchers: rule.Labels, StartsAt: time.Now().Add(-time.Minute), EndsAt: time.Now().Add(time.Minute),
				})
				if err != nil {
					t.Fatal(err)
				}
				silenceID = silence.ID
			}
			deps.EvaluateSingleRule(ctx, rule, nil)
			deps.WaitForNotificationDispatches()
			old, found, err := deps.AlertInstanceStore.GetActiveInstanceByFingerprint(rule.ID, alerts.GenerateFingerprint(rule.ID, rule.Labels))
			if err != nil || !found {
				t.Fatalf("first alert missing: found=%v err=%v", found, err)
			}
			if initial == alerts.InstanceStatusAcknowledged {
				rec := httptest.NewRecorder()
				deps.HandleAlertInstanceActions(rec, httptest.NewRequest(http.MethodPost, "/alerts/instances/"+old.ID+"/ack", nil))
				if rec.Code != http.StatusOK {
					t.Fatalf("acknowledge: %d %s", rec.Code, rec.Body.String())
				}
			}
			if silenceID != "" {
				if old.Status != alerts.InstanceStatusPending || len(adapter.snapshot()) != 0 {
					t.Fatal("silenced condition should remain pending without delivery")
				}
				// Removing the silence before recovery must not invent a recovered
				// notification for an alert that never fired.
				if err := deps.AlertInstanceStore.DeleteAlertSilence(silenceID); err != nil {
					t.Fatal(err)
				}
			}
			failureStore := &recoveryFailureStore{AlertInstanceStore: deps.AlertInstanceStore, AlertInstanceContextStore: deps.AlertInstanceStore.(persistence.AlertInstanceContextStore), fail: initial == "write_failure"}
			deps.AlertInstanceStore = failureStore
			rule.Condition["threshold"] = float64(100)
			if failureStore.fail {
				beforeCalls, beforeBroadcasts := len(adapter.snapshot()), len(broadcasts)
				deps.EvaluateSingleRule(ctx, rule, nil)
				deps.WaitForNotificationDispatches()
				stillActive, ok, loadErr := deps.AlertInstanceStore.GetAlertInstance(old.ID)
				if loadErr != nil || !ok || stillActive.Status != alerts.InstanceStatusFiring || stillActive.ResolvedAt != nil {
					t.Fatalf("failed write lost retryable firing state: %+v %v", stillActive, loadErr)
				}
				if len(adapter.snapshot()) != beforeCalls || len(broadcasts) != beforeBroadcasts {
					t.Fatal("failed recovery write emitted a false recovery")
				}
				failureStore.fail = false
			}
			deps.EvaluateSingleRule(ctx, rule, nil)
			deps.WaitForNotificationDispatches()
			resolved, ok, err := deps.AlertInstanceStore.GetAlertInstance(old.ID)
			if err != nil || !ok || resolved.Status != alerts.InstanceStatusResolved || resolved.ResolvedAt == nil {
				t.Fatalf("healthy condition did not resolve old instance: %+v %v", resolved, err)
			}
			if _, active, err := deps.AlertInstanceStore.GetActiveInstanceByFingerprint(rule.ID, old.Fingerprint); err != nil || active {
				t.Fatalf("healthy condition retained active fingerprint: %v %v", active, err)
			}
			// A repeated healthy evaluation sends no duplicate recovery. The next
			// actual breach must produce a fresh firing instance and notification.
			deps.EvaluateSingleRule(ctx, rule, nil)
			deps.WaitForNotificationDispatches()
			rule.Condition["threshold"] = float64(80)
			deps.EvaluateSingleRule(ctx, rule, nil)
			deps.WaitForNotificationDispatches()
			fresh, found, err := deps.AlertInstanceStore.GetActiveInstanceByFingerprint(rule.ID, old.Fingerprint)
			if err != nil || !found || fresh.ID == old.ID || fresh.Status != alerts.InstanceStatusFiring {
				t.Fatalf("recurrence did not fire a new instance: %+v %v", fresh, err)
			}
			wantStates := []string{"firing", "resolved", "firing"}
			wantBroadcasts := []string{"alert.fired", "alert.resolved", "alert.fired"}
			if initial == alerts.InstanceStatusPending {
				wantStates, wantBroadcasts = []string{"firing"}, []string{"alert.fired"}
			}
			var gotStates []string
			for _, call := range adapter.snapshot() {
				gotStates = append(gotStates, call.payload["state"].(string))
			}
			if !reflect.DeepEqual(gotStates, wantStates) || !reflect.DeepEqual(broadcasts, wantBroadcasts) {
				t.Fatalf("notifications=%v broadcasts=%v; want %v / %v", gotStates, broadcasts, wantStates, wantBroadcasts)
			}
		})
	}
}

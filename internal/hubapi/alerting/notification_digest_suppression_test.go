package alerting

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/hubapi/groupfeatures"
	"github.com/labtether/labtether/internal/notifications"
	"github.com/labtether/labtether/internal/persistence"
)

func digestSuppressionFixture(t *testing.T) (*Deps, *durableDigestPushStoreStub, *apnsFanoutAdapterStub, persistence.PushDigestClaim) {
	t.Helper()
	d := newTestAlertingDeps(t)
	device := digestEnabledDevice()
	store := &durableDigestPushStoreStub{}
	channels := newNotificationSecurityStore()
	channels.seed(digestChannel())
	adapter := &apnsFanoutAdapterStub{}
	d.NotificationStore = channels
	d.EvaluateGuardrailsContext = digestAllowGuardrails
	d.PushDeviceStore = store
	d.NotificationAdapters = map[string]notifications.Adapter{notifications.ChannelTypeAPNs: adapter}
	claim := persistence.PushDigestClaim{Device: device, ChannelID: digestChannel().ID, DeliveryGeneration: 1, ExpiresAt: time.Now().Add(time.Hour), Events: []persistence.PushDigestEvent{
		{ID: "blocked", RuleID: "rule-blocked", Labels: map[string]string{"team": "quiet"}, Severity: "warning", GroupIDs: []string{"group-blocked"}, MaintenanceScopeComplete: true},
		{ID: "allowed", RuleID: "rule-allowed", Labels: map[string]string{}, Severity: "warning", MaintenanceScopeComplete: true},
	}}
	return d, store, adapter, claim
}

func TestPushDigestFiltersCurrentSilenceAndMaintenancePerEntry(t *testing.T) {
	for _, kind := range []string{"silence", "maintenance"} {
		t.Run(kind, func(t *testing.T) {
			d, store, adapter, claim := digestSuppressionFixture(t)
			var silenceID string
			maintenance := kind == "maintenance"
			if kind == "silence" {
				silenceID = silenceForDelivery(t, d, map[string]string{"team": "quiet"})
			}
			d.EvaluateGuardrailsContext = func(_ context.Context, group string, _ time.Time) (groupfeatures.GroupMaintenanceGuardrails, error) {
				return groupfeatures.GroupMaintenanceGuardrails{SuppressAlerts: maintenance && group == "group-blocked"}, nil
			}
			store.claims = []persistence.PushDigestClaim{claim}
			d.ProcessDuePushDigests(context.Background())
			if len(adapter.calls) != 1 || adapter.calls[0].payload["digest_count"] != 1 {
				t.Fatalf("unrelated entry did not send alone: %+v", adapter.calls)
			}
			first := store.settlements[0]
			if !reflect.DeepEqual(first.Sent, []string{"allowed"}) || !reflect.DeepEqual(first.Deferred, []string{"blocked"}) || len(first.Discarded)+len(first.RetryAt) != 0 {
				t.Fatalf("bad partition: %+v", first)
			}
			if !first.DeferredUntil.After(time.Now()) {
				t.Fatal("deferral is not bounded in the future")
			}
			firstCollapse := adapter.calls[0].payload["collapse_id"]
			for _, key := range []string{"labels", "rule_id", "group_ids"} {
				if _, ok := adapter.calls[0].payload[key]; ok {
					t.Fatalf("APNs exposed %s", key)
				}
			}
			if silenceID != "" {
				if err := d.AlertInstanceStore.DeleteAlertSilence(silenceID); err != nil {
					t.Fatal(err)
				}
			}
			maintenance = false
			claim.Events = claim.Events[:1]
			claim.DeliveryGeneration++
			store.claims = []persistence.PushDigestClaim{claim}
			d.ProcessDuePushDigests(context.Background())
			if len(adapter.calls) != 2 || adapter.calls[1].payload["digest_count"] != 1 || adapter.calls[1].payload["collapse_id"] == firstCollapse {
				t.Fatal("deferred subset was lost or reused the unrelated collapse ID")
			}
			if !reflect.DeepEqual(store.settlements[1].Sent, []string{"blocked"}) {
				t.Fatalf("previously sent entry replayed: %+v", store.settlements[1])
			}
		})
	}
}

func TestPushDigestDefersUnknownIdentityAndPreservesBlockedRetryBudget(t *testing.T) {
	for _, kind := range []string{"legacy", "error", "full_page", "exhausted"} {
		t.Run(kind, func(t *testing.T) {
			d, store, adapter, claim := digestSuppressionFixture(t)
			switch kind {
			case "legacy":
				claim.Events[0].RuleID = ""
				claim.Events[0].Labels = nil
			case "error", "full_page":
				d.AlertInstanceStore = &unavailableSilenceStore{AlertInstanceStore: d.AlertInstanceStore, fullPage: kind == "full_page"}
			case "exhausted":
				silenceForDelivery(t, d, map[string]string{"team": "quiet"})
				claim.Events[1].RetryCount = pushDigestMaxRetries - 1
				adapter.groupFailures = map[string]int{"com.labtether.mobile/production": 1}
			}
			store.claims = []persistence.PushDigestClaim{claim}
			d.ProcessDuePushDigests(context.Background())
			if len(store.settlements) != 1 {
				t.Fatal("claim was not settled")
			}
			got := store.settlements[0]
			if kind == "error" || kind == "full_page" {
				if len(adapter.calls) != 0 || len(got.Deferred) != 2 || len(got.RetryAt)+len(got.Discarded)+len(got.Sent) != 0 {
					t.Fatalf("uncertain silence leaked or spent retry: %+v", got)
				}
			} else {
				if !reflect.DeepEqual(got.Deferred, []string{"blocked"}) || len(got.RetryAt) != 0 {
					t.Fatalf("blocked entry budget changed: %+v", got)
				}
				if kind == "exhausted" && !reflect.DeepEqual(got.Discarded, []string{"allowed"}) {
					t.Fatalf("exhaustion retired wrong entries: %+v", got)
				}
				if kind == "legacy" && !reflect.DeepEqual(got.Sent, []string{"allowed"}) {
					t.Fatalf("legacy entry blocked unrelated send: %+v", got)
				}
			}
		})
	}
}

func TestAPNsInvalidDigestIdentityNeverFallsBackToImmediateSend(t *testing.T) {
	for _, kind := range []string{"missing", "oversized", "empty_valid"} {
		t.Run(kind, func(t *testing.T) {
			d, store, adapter, _ := digestSuppressionFixture(t)
			store.devices = []persistence.PushDevice{digestEnabledDevice()}
			payload := map[string]any{"event": "alert.firing", "alert_id": "event", "rule_id": "rule", "state": "firing", "severity": "warning"}
			if kind == "oversized" {
				payload["labels"] = map[string]string{"team": strings.Repeat("a", 4097)}
			}
			if kind == "empty_valid" {
				payload["labels"] = map[string]string{}
			}
			err := d.sendNotification(context.Background(), digestChannel(), payload)
			if len(adapter.calls) != 0 {
				t.Fatal("digest failure became immediate APNs send")
			}
			if kind == "empty_valid" {
				if err != nil || len(store.enqueueBatches) != 1 || store.enqueueBatches[0][0].Labels == nil {
					t.Fatalf("valid empty labels lost: %v", err)
				}
			} else if err == nil || len(store.enqueueBatches) != 0 {
				t.Fatalf("invalid identity was not rejected: %v", err)
			}
		})
	}
}

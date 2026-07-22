package alerting

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/hubapi/groupfeatures"
	"github.com/labtether/labtether/internal/notifications"
	"github.com/labtether/labtether/internal/persistence"
)

type durableDigestPushStoreStub struct {
	mu sync.Mutex

	devices        []persistence.PushDevice
	enqueueBatches [][]persistence.PushDigestEnqueue
	enqueueResult  persistence.PushDigestEnqueueResult
	enqueueErr     error
	claims         []persistence.PushDigestClaim
	completed      [][]string
	released       []digestReleaseCall
	deleted        [][3]string
}

type digestReleaseCall struct {
	deviceID       string
	generation     int64
	nextAttempt    time.Time
	incrementRetry bool
}

func (s *durableDigestPushStoreStub) GetAllPushTokens(context.Context) ([]persistence.PushDevice, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]persistence.PushDevice(nil), s.devices...), nil
}

func (s *durableDigestPushStoreStub) DeletePushDeviceByToken(_ context.Context, token, bundleID, environment string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleted = append(s.deleted, [3]string{token, bundleID, environment})
	return nil
}

func (s *durableDigestPushStoreStub) EnqueuePushDigestEvents(_ context.Context, events []persistence.PushDigestEnqueue) (persistence.PushDigestEnqueueResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	copyEvents := append([]persistence.PushDigestEnqueue(nil), events...)
	s.enqueueBatches = append(s.enqueueBatches, copyEvents)
	return s.enqueueResult, s.enqueueErr
}

func (s *durableDigestPushStoreStub) ClaimDuePushDigests(context.Context, time.Time, time.Duration, int) ([]persistence.PushDigestClaim, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	claims := append([]persistence.PushDigestClaim(nil), s.claims...)
	s.claims = nil
	return claims, nil
}

func (s *durableDigestPushStoreStub) CompletePushDigestClaim(_ context.Context, _ string, _ int64, eventIDs []string, _ time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.completed = append(s.completed, append([]string(nil), eventIDs...))
	return true, nil
}

func (s *durableDigestPushStoreStub) ReleasePushDigestClaim(_ context.Context, deviceID string, generation int64, nextAttempt time.Time, incrementRetry bool) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.released = append(s.released, digestReleaseCall{
		deviceID: deviceID, generation: generation, nextAttempt: nextAttempt, incrementRetry: incrementRetry,
	})
	return true, nil
}

func digestEnabledDevice() persistence.PushDevice {
	device := enabledPushDevice("digest-token", "com.labtether.mobile", "production")
	device.ID = "push-device-1"
	device.DeviceID = "ios-install-1"
	device.PushCategory = "all_alerts"
	device.MinimumSeverity = "warning"
	device.DigestWindowSeconds = 180
	device.TimeZone = "UTC"
	return device
}

func digestChannel() notifications.Channel {
	return notifications.Channel{
		ID: "apns-digest-channel", Name: "iOS push", Type: notifications.ChannelTypeAPNs, Enabled: true,
		Config: map[string]any{
			"bundle_id": "com.labtether.mobile", "production": true,
		},
	}
}

func TestAPNsNonUrgentAlertQueuesDurableDigestAndHighRemainsImmediate(t *testing.T) {
	store := &durableDigestPushStoreStub{
		devices:       []persistence.PushDevice{digestEnabledDevice()},
		enqueueResult: persistence.PushDigestEnqueueResult{Inserted: 1},
	}
	adapter := &apnsFanoutAdapterStub{}
	deps := &Deps{
		PushDeviceStore: store,
		NotificationAdapters: map[string]notifications.Adapter{
			notifications.ChannelTypeAPNs: adapter,
		},
	}

	warning := map[string]any{
		"event": "alert.firing", "alert_id": "alert-warning", "state": "firing",
		"rule_id": "rule-1", "severity": "warning", "group_ids": []string{"group-a"},
		"title": "private alert title", "text": "private alert detail",
	}
	if err := deps.sendNotification(context.Background(), digestChannel(), warning); err != nil {
		t.Fatalf("queue warning digest: %v", err)
	}
	if len(adapter.calls) != 0 {
		t.Fatalf("warning alert was sent immediately: %+v", adapter.calls)
	}
	if len(store.enqueueBatches) != 1 || len(store.enqueueBatches[0]) != 1 {
		t.Fatalf("digest enqueue batches = %+v", store.enqueueBatches)
	}
	enqueued := store.enqueueBatches[0][0]
	if enqueued.DeviceID != "push-device-1" || enqueued.ChannelID != digestChannel().ID || enqueued.WindowSeconds != 180 {
		t.Fatalf("unexpected digest routing snapshot: %+v", enqueued)
	}
	if enqueued.DedupeKey == "" || enqueued.Severity != "warning" || len(enqueued.GroupIDs) != 1 {
		t.Fatalf("unexpected privacy-minimised digest event: %+v", enqueued)
	}

	high := map[string]any{
		"event": "alert.firing", "alert_id": "alert-high", "state": "firing",
		"rule_id": "rule-2", "severity": "high", "title": "High alert", "text": "Immediate",
	}
	if err := deps.sendNotification(context.Background(), digestChannel(), high); err != nil {
		t.Fatalf("send high alert: %v", err)
	}
	if len(adapter.calls) != 1 {
		t.Fatalf("high alert adapter calls = %d, want 1", len(adapter.calls))
	}
	if len(store.enqueueBatches) != 1 {
		t.Fatalf("high alert unexpectedly entered digest queue: %+v", store.enqueueBatches)
	}
}

func TestAPNsNonUrgentAlertQueuesDuringQuietHours(t *testing.T) {
	device := digestEnabledDevice()
	device.QuietHoursEnabled = true
	device.TimeZone = "UTC"
	currentMinutes := time.Now().UTC().Hour()*60 + time.Now().UTC().Minute()
	device.QuietHoursStartMinutes = (currentMinutes + 1439) % 1440
	device.QuietHoursEndMinutes = (currentMinutes + 2) % 1440
	store := &durableDigestPushStoreStub{
		devices:       []persistence.PushDevice{device},
		enqueueResult: persistence.PushDigestEnqueueResult{Inserted: 1},
	}
	adapter := &apnsFanoutAdapterStub{}
	deps := &Deps{
		PushDeviceStore: store,
		NotificationAdapters: map[string]notifications.Adapter{
			notifications.ChannelTypeAPNs: adapter,
		},
	}
	payload := map[string]any{
		"event": "alert.firing", "alert_id": "alert-quiet", "state": "firing",
		"rule_id": "rule-quiet", "severity": "warning", "title": "private", "text": "private",
	}

	if err := deps.sendNotification(context.Background(), digestChannel(), payload); err != nil {
		t.Fatalf("queue quiet-hours digest: %v", err)
	}
	if len(store.enqueueBatches) != 1 || len(store.enqueueBatches[0]) != 1 {
		t.Fatalf("quiet-hours alert was lost before durable enqueue: %+v", store.enqueueBatches)
	}
	if len(adapter.calls) != 0 {
		t.Fatalf("quiet-hours alert was sent immediately: %+v", adapter.calls)
	}
}

func TestPushDigestLeaseCoversClaimedWorkerQueue(t *testing.T) {
	waves := (pushDigestClaimLimit + pushDigestWorkerLimit - 1) / pushDigestWorkerLimit
	minimum := time.Duration(waves) * notificationDispatchTimeout
	if pushDigestLeaseDuration <= minimum {
		t.Fatalf("digest lease %s must exceed %d worst-case worker waves (%s)", pushDigestLeaseDuration, waves, minimum)
	}
}

func TestAPNsFullDigestQueueReturnsTargetedRetryWithoutImmediateLeak(t *testing.T) {
	device := digestEnabledDevice()
	store := &durableDigestPushStoreStub{
		devices: []persistence.PushDevice{device},
		enqueueResult: persistence.PushDigestEnqueueResult{
			Dropped: 1, DroppedDeviceIDs: []string{device.ID},
		},
	}
	adapter := &apnsFanoutAdapterStub{}
	deps := &Deps{
		PushDeviceStore: store,
		NotificationAdapters: map[string]notifications.Adapter{
			notifications.ChannelTypeAPNs: adapter,
		},
	}
	payload := map[string]any{
		"event": "alert.firing", "alert_id": "alert-cap", "state": "firing",
		"severity": "warning", "title": "warning", "text": "detail",
	}
	err := deps.sendNotification(context.Background(), digestChannel(), payload)
	if err == nil {
		t.Fatal("full digest queue unexpectedly succeeded")
	}
	if len(adapter.calls) != 0 {
		t.Fatal("queue pressure failed open to an immediate non-urgent push")
	}
	retryPayload, targeted := payloadWithAPNsRetryTargets(payload, err)
	if !targeted {
		t.Fatalf("queue pressure did not retain a targeted retry: %v", err)
	}
	targets, restricted, parseErr := apnsRetryTargetSetFromPayload(retryPayload)
	if parseErr != nil || !restricted || len(targets) != 1 {
		t.Fatalf("invalid targeted retry metadata: targets=%+v restricted=%v err=%v", targets, restricted, parseErr)
	}
}

func TestProcessDuePushDigestSendsOnePrivacySafePerDeviceSummary(t *testing.T) {
	device := digestEnabledDevice()
	device.MinimumSeverity = "info"
	store := &durableDigestPushStoreStub{claims: []persistence.PushDigestClaim{{
		Device: device, ChannelID: digestChannel().ID, WindowSeconds: 180,
		DeliveryGeneration: 4, ExpiresAt: time.Now().Add(time.Hour),
		Events: []persistence.PushDigestEvent{
			{ID: "event-1", Severity: "warning", GroupIDs: []string{"group-a"}, MaintenanceScopeComplete: true},
			{ID: "event-2", Severity: "info", GroupIDs: []string{"group-b"}, MaintenanceScopeComplete: true},
		},
	}}}
	channelStore := newNotificationSecurityStore()
	channelStore.seed(digestChannel())
	adapter := &apnsFanoutAdapterStub{}
	deps := &Deps{
		NotificationStore: channelStore,
		PushDeviceStore:   store,
		NotificationAdapters: map[string]notifications.Adapter{
			notifications.ChannelTypeAPNs: adapter,
		},
	}

	deps.ProcessDuePushDigests(context.Background())
	if len(adapter.calls) != 1 {
		t.Fatalf("digest APNs calls = %d, want 1", len(adapter.calls))
	}
	call := adapter.calls[0]
	if got := call.payload["digest_count"]; got != 2 {
		t.Fatalf("digest_count = %#v, want 2", got)
	}
	if call.payload["event"] != "alert.digest" || call.payload["severity"] != "warning" {
		t.Fatalf("unexpected digest payload: %+v", call.payload)
	}
	for _, forbidden := range []string{"alert_id", "alert_instance_id", "rule_id", "rule_name", "description", "group_ids"} {
		if _, present := call.payload[forbidden]; present {
			t.Fatalf("digest payload exposed %s: %+v", forbidden, call.payload)
		}
	}
	if tokens, _ := call.config["device_tokens"].([]string); len(tokens) != 1 || tokens[0] != device.PushToken {
		t.Fatalf("digest target tokens = %+v", tokens)
	}
	if len(store.completed) != 1 || len(store.completed[0]) != 2 {
		t.Fatalf("completed snapshots = %+v", store.completed)
	}
	if len(store.released) != 0 {
		t.Fatalf("successful digest was rescheduled: %+v", store.released)
	}
}

func TestProcessDuePushDigestRechecksPreferencesQuietHoursAndMaintenance(t *testing.T) {
	baseClaim := func(device persistence.PushDevice) persistence.PushDigestClaim {
		return persistence.PushDigestClaim{
			Device: device, ChannelID: digestChannel().ID, DeliveryGeneration: 1,
			ExpiresAt: time.Now().Add(time.Hour),
			Events: []persistence.PushDigestEvent{{
				ID: "event-1", Severity: "warning", GroupIDs: []string{"group-a"}, MaintenanceScopeComplete: true,
			}},
		}
	}

	t.Run("preference downgrade discards", func(t *testing.T) {
		device := digestEnabledDevice()
		device.PushCategory = "critical_only"
		store := &durableDigestPushStoreStub{claims: []persistence.PushDigestClaim{baseClaim(device)}}
		deps := &Deps{NotificationStore: newNotificationSecurityStore(), PushDeviceStore: store}
		deps.ProcessDuePushDigests(context.Background())
		if len(store.completed) != 1 || len(store.released) != 0 {
			t.Fatalf("preference recheck complete=%+v release=%+v", store.completed, store.released)
		}
	})

	t.Run("quiet hours defers without retry", func(t *testing.T) {
		device := digestEnabledDevice()
		device.QuietHoursEnabled = true
		device.QuietHoursStartMinutes = 0
		device.QuietHoursEndMinutes = 1439
		store := &durableDigestPushStoreStub{claims: []persistence.PushDigestClaim{baseClaim(device)}}
		deps := &Deps{NotificationStore: newNotificationSecurityStore(), PushDeviceStore: store}
		deps.ProcessDuePushDigests(context.Background())
		if len(store.released) != 1 || store.released[0].incrementRetry || len(store.completed) != 0 {
			t.Fatalf("quiet recheck complete=%+v release=%+v", store.completed, store.released)
		}
	})

	t.Run("invalid quiet hours timezone fails closed", func(t *testing.T) {
		device := digestEnabledDevice()
		device.QuietHoursEnabled = true
		device.TimeZone = ""
		store := &durableDigestPushStoreStub{claims: []persistence.PushDigestClaim{baseClaim(device)}}
		deps := &Deps{NotificationStore: newNotificationSecurityStore(), PushDeviceStore: store}
		deps.ProcessDuePushDigests(context.Background())
		if len(store.released) != 1 || store.released[0].incrementRetry || len(store.completed) != 0 {
			t.Fatalf("invalid timezone recheck complete=%+v release=%+v", store.completed, store.released)
		}
	})

	t.Run("maintenance defers without retry", func(t *testing.T) {
		device := digestEnabledDevice()
		store := &durableDigestPushStoreStub{claims: []persistence.PushDigestClaim{baseClaim(device)}}
		deps := &Deps{
			NotificationStore: newNotificationSecurityStore(), PushDeviceStore: store,
			EvaluateGuardrails: func(groupID string, _ time.Time) (groupfeatures.GroupMaintenanceGuardrails, error) {
				if groupID != "group-a" {
					return groupfeatures.GroupMaintenanceGuardrails{}, errors.New("unexpected group")
				}
				return groupfeatures.GroupMaintenanceGuardrails{SuppressAlerts: true}, nil
			},
		}
		deps.ProcessDuePushDigests(context.Background())
		if len(store.released) != 1 || store.released[0].incrementRetry || len(store.completed) != 0 {
			t.Fatalf("maintenance recheck complete=%+v release=%+v", store.completed, store.released)
		}
	})
}

func TestProcessDuePushDigestRetriesBoundedlyOnAPNsFailure(t *testing.T) {
	device := digestEnabledDevice()
	claim := persistence.PushDigestClaim{
		Device: device, ChannelID: digestChannel().ID, DeliveryGeneration: 7,
		RetryCount: 1, ExpiresAt: time.Now().Add(time.Hour),
		Events: []persistence.PushDigestEvent{{ID: "event-1", Severity: "warning", MaintenanceScopeComplete: true}},
	}
	store := &durableDigestPushStoreStub{claims: []persistence.PushDigestClaim{claim}}
	channelStore := newNotificationSecurityStore()
	channelStore.seed(digestChannel())
	adapter := &apnsFanoutAdapterStub{groupFailures: map[string]int{"com.labtether.mobile/production": 1}}
	deps := &Deps{
		NotificationStore: channelStore, PushDeviceStore: store,
		NotificationAdapters: map[string]notifications.Adapter{notifications.ChannelTypeAPNs: adapter},
	}
	deps.ProcessDuePushDigests(context.Background())
	if len(store.released) != 1 || !store.released[0].incrementRetry {
		t.Fatalf("failed digest release = %+v", store.released)
	}
	if len(store.completed) != 0 {
		t.Fatalf("transient failure prematurely discarded: %+v", store.completed)
	}
}

func TestPushDigestCollapseIDIsStableAcrossLeaseReclaim(t *testing.T) {
	device := digestEnabledDevice()
	events := []persistence.PushDigestEvent{{
		ID: "oldest-event", Severity: "warning", MaintenanceScopeComplete: true,
	}}
	first := buildPushDigestPayload(events, persistence.PushDigestClaim{
		Device: device, DeliveryGeneration: 1, Events: events,
	})
	reclaimed := buildPushDigestPayload(events, persistence.PushDigestClaim{
		Device: device, DeliveryGeneration: 2, Events: events,
	})
	if first["collapse_id"] != reclaimed["collapse_id"] {
		t.Fatalf("lease reclaim changed collapse id: first=%v reclaimed=%v", first["collapse_id"], reclaimed["collapse_id"])
	}
	nextEvents := []persistence.PushDigestEvent{{
		ID: "next-event", Severity: "warning", MaintenanceScopeComplete: true,
	}}
	next := buildPushDigestPayload(nextEvents, persistence.PushDigestClaim{
		Device: device, DeliveryGeneration: 3, Events: nextEvents,
	})
	if first["collapse_id"] == next["collapse_id"] {
		t.Fatalf("later digest reused prior collapse id: %v", next["collapse_id"])
	}
}

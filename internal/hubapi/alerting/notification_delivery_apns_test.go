package alerting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/notifications"
	"github.com/labtether/labtether/internal/persistence"
)

type apnsFanoutStoreStub struct {
	devices []persistence.PushDevice
	err     error
	deleted [][3]string
}

func (s *apnsFanoutStoreStub) GetAllPushTokens(context.Context) ([]persistence.PushDevice, error) {
	return append([]persistence.PushDevice(nil), s.devices...), s.err
}

func (s *apnsFanoutStoreStub) DeletePushDeviceByToken(_ context.Context, token, bundleID, environment string) error {
	s.deleted = append(s.deleted, [3]string{token, bundleID, environment})
	return nil
}

type apnsFanoutAdapterStub struct {
	calls                 []apnsFanoutCall
	rejectFirstTokenAsBad bool
	tokenFailures         map[string]int
	groupFailures         map[string]int
}

type apnsFanoutCall struct {
	config  map[string]any
	payload map[string]any
}

type apnsRetryNotificationStore struct{ *notificationSecurityStore }

func (s *apnsRetryNotificationStore) ListPendingRetries(_ context.Context, now time.Time, limit int) ([]notifications.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 {
		limit = 50
	}
	records := make([]notifications.Record, 0, limit)
	for _, record := range s.records {
		if record.Status != notifications.RecordStatusFailed || record.NextRetryAt == nil || record.NextRetryAt.After(now) {
			continue
		}
		if record.RetryCount >= record.MaxRetries {
			continue
		}
		copyRecord := record
		copyRecord.Payload = deepCloneNotificationMap(record.Payload)
		records = append(records, copyRecord)
		if len(records) == limit {
			break
		}
	}
	return records, nil
}

func (a *apnsFanoutAdapterStub) Type() string { return notifications.ChannelTypeAPNs }

func (a *apnsFanoutAdapterStub) Send(_ context.Context, config map[string]any, payload map[string]any) error {
	a.calls = append(a.calls, apnsFanoutCall{
		config:  cloneAnyMap(config),
		payload: cloneAnyMap(payload),
	})
	if a.rejectFirstTokenAsBad {
		if tokens, ok := config["device_tokens"].([]string); ok && len(tokens) > 0 {
			for _, value := range config {
				if handler, ok := value.(notifications.APNsInvalidDeviceTokenHandler); ok {
					_ = handler(tokens[0])
					break
				}
			}
		}
	}
	tokens, _ := config["device_tokens"].([]string)
	failedIndices := make([]int, 0)
	for index, token := range tokens {
		if a.tokenFailures[token] > 0 {
			a.tokenFailures[token]--
			failedIndices = append(failedIndices, index)
		}
	}
	if len(failedIndices) > 0 {
		return &apnsIndexedFailureStub{indices: failedIndices}
	}
	bundleID, _ := config["bundle_id"].(string)
	environment := "sandbox"
	if production, _ := config["production"].(bool); production {
		environment = "production"
	}
	groupKey := bundleID + "/" + environment
	if a.groupFailures[groupKey] > 0 {
		a.groupFailures[groupKey]--
		return errors.New("synthetic APNs group outage")
	}
	return nil
}

type apnsIndexedFailureStub struct{ indices []int }

func (e *apnsIndexedFailureStub) Error() string { return "synthetic indexed APNs delivery failure" }

func (e *apnsIndexedFailureStub) FailedDeliveryIndices() []int {
	return append([]int(nil), e.indices...)
}

func enabledPushDevice(token, bundleID, environment string) persistence.PushDevice {
	return persistence.PushDevice{
		Platform:             "ios",
		PushToken:            token,
		BundleID:             bundleID,
		Environment:          environment,
		TimeZone:             "UTC",
		NotifyCriticalAlerts: true,
		NotifyNodeOffline:    true,
		NotifyServiceDown:    true,
		PushCategory:         "all_alerts",
		MinimumSeverity:      "warning",
	}
}

func TestAPNsNotificationTestReachesDefaultEligibleDevice(t *testing.T) {
	store := &apnsFanoutStoreStub{devices: []persistence.PushDevice{
		enabledPushDevice("default-device", "com.labtether.mobile", "sandbox"),
	}}
	adapter := &apnsFanoutAdapterStub{}
	deps := &Deps{
		PushDeviceStore: store,
		NotificationAdapters: map[string]notifications.Adapter{
			notifications.ChannelTypeAPNs: adapter,
		},
	}
	channel := notifications.Channel{Type: notifications.ChannelTypeAPNs, Config: map[string]any{
		"bundle_id": "com.labtether.mobile",
	}}
	payload := map[string]any{
		"event":             "alert.test",
		"severity":          "critical",
		"state":             "firing",
		"notification_test": true,
		"alert_id":          "notification-channel-test",
	}

	if err := deps.sendNotification(context.Background(), channel, payload); err != nil {
		t.Fatalf("send APNs test: %v", err)
	}
	if len(adapter.calls) != 1 {
		t.Fatalf("adapter calls = %d, want 1", len(adapter.calls))
	}
	if tokens, _ := adapter.calls[0].config["device_tokens"].([]string); len(tokens) != 1 || tokens[0] != "default-device" {
		t.Fatalf("APNs test tokens = %v, want default-device", tokens)
	}
}

func TestAPNsNotificationTestFailsWhenNoEligibleDeviceExists(t *testing.T) {
	adapter := &apnsFanoutAdapterStub{}
	deps := &Deps{
		PushDeviceStore: &apnsFanoutStoreStub{},
		NotificationAdapters: map[string]notifications.Adapter{
			notifications.ChannelTypeAPNs: adapter,
		},
	}
	channel := notifications.Channel{Type: notifications.ChannelTypeAPNs, Config: map[string]any{
		"bundle_id": "com.labtether.mobile",
	}}
	err := deps.sendNotification(context.Background(), channel, map[string]any{
		"event":             "alert.test",
		"severity":          "critical",
		"state":             "firing",
		"notification_test": true,
	})
	if err == nil || !strings.Contains(err.Error(), "no eligible registered APNs devices") {
		t.Fatalf("expected no-eligible-device failure, got %v", err)
	}
	if len(adapter.calls) != 0 {
		t.Fatalf("adapter called without eligible devices: %d", len(adapter.calls))
	}
}

func TestSendAPNsNotificationProvidesPermanentTokenCleanupHandler(t *testing.T) {
	store := &apnsFanoutStoreStub{devices: []persistence.PushDevice{
		enabledPushDevice("invalid-token", "com.labtether.mobile", "production"),
	}}
	adapter := &apnsFanoutAdapterStub{rejectFirstTokenAsBad: true}
	deps := &Deps{
		PushDeviceStore: store,
		NotificationAdapters: map[string]notifications.Adapter{
			notifications.ChannelTypeAPNs: adapter,
		},
	}
	channel := notifications.Channel{Type: notifications.ChannelTypeAPNs, Config: map[string]any{
		"bundle_id": "com.labtether.mobile", "production": true,
	}}

	if err := deps.sendNotification(context.Background(), channel, map[string]any{
		"event": "alert.firing", "alert_id": "a-1", "severity": "high",
	}); err != nil {
		t.Fatalf("sendNotification: %v", err)
	}
	if len(store.deleted) != 1 || store.deleted[0] != [3]string{"invalid-token", "com.labtether.mobile", "production"} {
		t.Fatalf("unexpected permanent-token cleanup: %+v", store.deleted)
	}
}

func TestSendAPNsNotificationPartitionsRegisteredDevicesByTopicAndEnvironment(t *testing.T) {
	devices := []persistence.PushDevice{
		enabledPushDevice("prod-main", "com.labtether.mobile", "production"),
		enabledPushDevice("sandbox-main", "com.labtether.mobile", "sandbox"),
		enabledPushDevice("sandbox-debug", "com.labtether.mobile.debug", "sandbox"),
		enabledPushDevice("sandbox-main", "com.labtether.mobile", "sandbox"), // duplicate token
		enabledPushDevice("too-low", "com.labtether.mobile", "production"),
		enabledPushDevice("disabled-critical", "com.labtether.mobile", "production"),
		enabledPushDevice("other-platform", "com.labtether.mobile", "production"),
	}
	devices[4].MinimumSeverity = "critical"
	devices[5].NotifyCriticalAlerts = false
	devices[6].Platform = "android"
	store := &apnsFanoutStoreStub{devices: devices}
	adapter := &apnsFanoutAdapterStub{}
	deps := &Deps{
		PushDeviceStore: store,
		NotificationAdapters: map[string]notifications.Adapter{
			notifications.ChannelTypeAPNs: adapter,
		},
	}
	channel := notifications.Channel{
		Type: notifications.ChannelTypeAPNs,
		Config: map[string]any{
			"auth_key_path": "/keys/AuthKey.p8",
			"key_id":        "KEY123",
			"team_id":       "TEAM123",
			"bundle_id":     "com.labtether.mobile",
			"production":    true,
			"device_tokens": []string{"stale-channel-token"},
			"allowed_bundle_ids": []string{
				"com.labtether.mobile",
				"com.labtether.mobile.debug",
			},
		},
	}
	payload := map[string]any{
		"event":    "alert.firing",
		"alert_id": "alert-1",
		"severity": "high",
	}

	if err := deps.sendNotification(context.Background(), channel, payload); err != nil {
		t.Fatalf("sendNotification: %v", err)
	}

	if len(adapter.calls) != 3 {
		t.Fatalf("adapter calls = %d, want 3: %+v", len(adapter.calls), adapter.calls)
	}
	got := make(map[string][]string, len(adapter.calls))
	for _, call := range adapter.calls {
		bundleID, _ := call.config["bundle_id"].(string)
		production, _ := call.config["production"].(bool)
		environment := "sandbox"
		if production {
			environment = "production"
		}
		tokens, ok := call.config["device_tokens"].([]string)
		if !ok {
			t.Fatalf("device_tokens type = %T, want []string", call.config["device_tokens"])
		}
		got[bundleID+"/"+environment] = tokens
	}
	assertStringSliceEqual(t, got["com.labtether.mobile/production"], []string{"prod-main"})
	assertStringSliceEqual(t, got["com.labtether.mobile/sandbox"], []string{"sandbox-main"})
	assertStringSliceEqual(t, got["com.labtether.mobile.debug/sandbox"], []string{"sandbox-debug"})
}

func TestAPNsPartialRetryNarrowsDurableTargetsAfterEveryAttempt(t *testing.T) {
	devices := []persistence.PushDevice{
		enabledPushDevice("token-a", "com.labtether.mobile", "production"),
		enabledPushDevice("token-b", "com.labtether.mobile", "production"),
		enabledPushDevice("token-c", "com.labtether.mobile", "production"),
	}
	for index := range devices {
		devices[index].ID = fmt.Sprintf("device-%d", index+1)
	}
	pushStore := &apnsFanoutStoreStub{devices: devices}
	adapter := &apnsFanoutAdapterStub{tokenFailures: map[string]int{
		"token-b": 1, // fails only during the initial fanout
		"token-c": 2, // fails initially and on the first retry
	}}
	historyStore := &apnsRetryNotificationStore{notificationSecurityStore: newNotificationSecurityStore()}
	channel := notifications.Channel{
		ID:      "apns-channel",
		Name:    "iOS push",
		Type:    notifications.ChannelTypeAPNs,
		Enabled: true,
		Config: map[string]any{
			"bundle_id":  "com.labtether.mobile",
			"production": true,
		},
	}
	historyStore.seed(channel)
	deps := &Deps{
		NotificationStore: historyStore,
		PushDeviceStore:   pushStore,
		NotificationAdapters: map[string]notifications.Adapter{
			notifications.ChannelTypeAPNs: adapter,
		},
	}
	payload := map[string]any{
		"event":    "alert.firing",
		"alert_id": "alert-partial",
		"severity": "high",
		"title":    "Partial delivery",
	}

	initialErr := deps.sendNotification(context.Background(), channel, payload)
	if initialErr == nil {
		t.Fatal("initial fanout unexpectedly succeeded")
	}
	for _, token := range []string{"token-a", "token-b", "token-c"} {
		if strings.Contains(initialErr.Error(), token) {
			t.Fatalf("fanout error exposed device token %q: %v", token, initialErr)
		}
	}
	retryPayload, targeted := payloadWithAPNsRetryTargets(payload, initialErr)
	if !targeted {
		t.Fatal("initial partial failure did not produce durable retry targets")
	}
	encodedRetryPayload, err := json.Marshal(retryPayload)
	if err != nil {
		t.Fatalf("marshal retry payload: %v", err)
	}
	for _, token := range []string{"token-a", "token-b", "token-c"} {
		if strings.Contains(string(encodedRetryPayload), token) {
			t.Fatalf("retry payload persisted raw APNs token %q: %s", token, encodedRetryPayload)
		}
	}
	var persistedRetryPayload map[string]any
	if err := json.Unmarshal(encodedRetryPayload, &persistedRetryPayload); err != nil {
		t.Fatalf("unmarshal persisted retry payload: %v", err)
	}
	record := deps.recordNotificationHistoryWithRetry(
		channel.ID,
		"alert-partial",
		"route-apns",
		notifications.RecordStatusFailed,
		initialErr.Error(),
		persistedRetryPayload,
	)
	if record.ID == "" {
		t.Fatal("failed to create partial-delivery history record")
	}

	setRecordDue := func() {
		historyStore.mu.Lock()
		defer historyStore.mu.Unlock()
		due := time.Now().UTC().Add(-time.Second)
		historyStore.records[0].NextRetryAt = &due
	}
	setRecordDue()
	deps.RetryPendingNotifications(context.Background())

	if len(adapter.calls) != 2 {
		t.Fatalf("adapter calls after first retry = %d, want 2", len(adapter.calls))
	}
	assertStringSliceEqual(t, adapter.calls[0].config["device_tokens"].([]string), []string{"token-a", "token-b", "token-c"})
	assertStringSliceEqual(t, adapter.calls[1].config["device_tokens"].([]string), []string{"token-b", "token-c"})

	historyStore.mu.Lock()
	firstRetryStatus := historyStore.records[0].Status
	firstRetryCount := historyStore.records[0].RetryCount
	remaining, restricted, parseErr := apnsRetryTargetSetFromPayload(historyStore.records[0].Payload)
	historyStore.mu.Unlock()
	if firstRetryStatus != notifications.RecordStatusFailed || firstRetryCount != 1 {
		t.Fatalf("first retry accounting = status %q count %d, want failed/1", firstRetryStatus, firstRetryCount)
	}
	if parseErr != nil || !restricted || len(remaining) != 1 {
		t.Fatalf("remaining retry targets = %d restricted=%t err=%v, want only token-c's registration", len(remaining), restricted, parseErr)
	}
	if _, ok := remaining[newAPNsRetryTarget(devices[2], devices[2].BundleID, devices[2].Environment)]; !ok {
		t.Fatalf("remaining retry target did not narrow to device-3: %+v", remaining)
	}

	setRecordDue()
	deps.RetryPendingNotifications(context.Background())
	if len(adapter.calls) != 3 {
		t.Fatalf("adapter calls after second retry = %d, want 3", len(adapter.calls))
	}
	assertStringSliceEqual(t, adapter.calls[2].config["device_tokens"].([]string), []string{"token-c"})

	historyStore.mu.Lock()
	defer historyStore.mu.Unlock()
	if historyStore.records[0].Status != notifications.RecordStatusSent || historyStore.records[0].RetryCount != 2 {
		t.Fatalf("final retry accounting = status %q count %d, want sent/2", historyStore.records[0].Status, historyStore.records[0].RetryCount)
	}
}

func TestAPNsPartialRetryStopsAtPersistedMaximumWithoutReplayingSuccesses(t *testing.T) {
	devices := []persistence.PushDevice{
		enabledPushDevice("success-token", "com.labtether.mobile", "production"),
		enabledPushDevice("failed-token", "com.labtether.mobile", "production"),
	}
	devices[0].ID = "success-device"
	devices[1].ID = "failed-device"
	pushStore := &apnsFanoutStoreStub{devices: devices}
	adapter := &apnsFanoutAdapterStub{tokenFailures: map[string]int{"failed-token": 10}}
	historyStore := &apnsRetryNotificationStore{notificationSecurityStore: newNotificationSecurityStore()}
	channel := notifications.Channel{
		ID:      "bounded-apns-channel",
		Name:    "Bounded iOS push",
		Type:    notifications.ChannelTypeAPNs,
		Enabled: true,
		Config: map[string]any{
			"bundle_id":  "com.labtether.mobile",
			"production": true,
		},
	}
	historyStore.seed(channel)
	deps := &Deps{
		NotificationStore: historyStore,
		PushDeviceStore:   pushStore,
		NotificationAdapters: map[string]notifications.Adapter{
			notifications.ChannelTypeAPNs: adapter,
		},
	}
	payload := map[string]any{"event": "alert.firing", "alert_id": "bounded", "severity": "high"}
	initialErr := deps.sendNotification(context.Background(), channel, payload)
	retryPayload, targeted := payloadWithAPNsRetryTargets(payload, initialErr)
	if initialErr == nil || !targeted {
		t.Fatalf("initial partial failure = %v targeted=%t", initialErr, targeted)
	}
	deps.recordNotificationHistoryWithRetry(
		channel.ID,
		"bounded",
		"route-apns",
		notifications.RecordStatusFailed,
		initialErr.Error(),
		retryPayload,
	)

	for attempt := 0; attempt < notifications.DefaultMaxRetries; attempt++ {
		historyStore.mu.Lock()
		due := time.Now().UTC().Add(-time.Second)
		historyStore.records[0].NextRetryAt = &due
		historyStore.mu.Unlock()
		deps.RetryPendingNotifications(context.Background())
	}
	callCountAfterExhaustion := len(adapter.calls)
	deps.RetryPendingNotifications(context.Background())
	if len(adapter.calls) != callCountAfterExhaustion {
		t.Fatalf("exhausted APNs retry dispatched again: calls %d -> %d", callCountAfterExhaustion, len(adapter.calls))
	}
	if len(adapter.calls) != 1+notifications.DefaultMaxRetries {
		t.Fatalf("adapter calls = %d, want initial + %d bounded retries", len(adapter.calls), notifications.DefaultMaxRetries)
	}
	assertStringSliceEqual(t, adapter.calls[0].config["device_tokens"].([]string), []string{"failed-token", "success-token"})
	for index, call := range adapter.calls[1:] {
		assertStringSliceEqual(t, call.config["device_tokens"].([]string), []string{"failed-token"})
		if strings.Contains(fmt.Sprintf("%v", call.config["device_tokens"]), "success-token") {
			t.Fatalf("retry %d replayed successful target", index+1)
		}
	}
	historyStore.mu.Lock()
	defer historyStore.mu.Unlock()
	if historyStore.records[0].Status != notifications.RecordStatusFailed ||
		historyStore.records[0].RetryCount != notifications.DefaultMaxRetries ||
		historyStore.records[0].NextRetryAt != nil {
		t.Fatalf("exhausted accounting = status %q count %d next=%v", historyStore.records[0].Status, historyStore.records[0].RetryCount, historyStore.records[0].NextRetryAt)
	}
}

func TestAPNsGroupFailureRetriesOnlyFailedTopicEnvironmentGroup(t *testing.T) {
	devices := []persistence.PushDevice{
		enabledPushDevice("prod-a", "com.labtether.mobile", "production"),
		enabledPushDevice("debug-a", "com.labtether.mobile.debug", "sandbox"),
		enabledPushDevice("debug-b", "com.labtether.mobile.debug", "sandbox"),
	}
	for index := range devices {
		devices[index].ID = fmt.Sprintf("group-device-%d", index+1)
	}
	store := &apnsFanoutStoreStub{devices: devices}
	adapter := &apnsFanoutAdapterStub{groupFailures: map[string]int{
		"com.labtether.mobile.debug/sandbox": 1,
	}}
	deps := &Deps{
		PushDeviceStore: store,
		NotificationAdapters: map[string]notifications.Adapter{
			notifications.ChannelTypeAPNs: adapter,
		},
	}
	channel := notifications.Channel{Type: notifications.ChannelTypeAPNs, Config: map[string]any{
		"bundle_id":          "com.labtether.mobile",
		"production":         true,
		"allowed_bundle_ids": []string{"com.labtether.mobile", "com.labtether.mobile.debug"},
	}}
	payload := map[string]any{"event": "alert.firing", "alert_id": "group-failure", "severity": "high"}

	initialErr := deps.sendNotification(context.Background(), channel, payload)
	if initialErr == nil {
		t.Fatal("expected one topic/environment group to fail")
	}
	retryPayload, targeted := payloadWithAPNsRetryTargets(payload, initialErr)
	if !targeted {
		t.Fatal("group failure did not produce retry targets")
	}
	if err := deps.sendNotification(context.Background(), channel, retryPayload); err != nil {
		t.Fatalf("targeted group retry: %v", err)
	}
	if len(adapter.calls) != 3 {
		t.Fatalf("adapter calls = %d, want two initial groups plus one retry", len(adapter.calls))
	}
	last := adapter.calls[len(adapter.calls)-1]
	if got := last.config["bundle_id"]; got != "com.labtether.mobile.debug" {
		t.Fatalf("retried bundle = %v, want failed debug bundle", got)
	}
	if production, _ := last.config["production"].(bool); production {
		t.Fatal("failed sandbox group was retried against production")
	}
	assertStringSliceEqual(t, last.config["device_tokens"].([]string), []string{"debug-a", "debug-b"})
}

func TestAPNsMalformedRetryRestrictionFailsClosedWithoutFleetFanout(t *testing.T) {
	store := &apnsFanoutStoreStub{devices: []persistence.PushDevice{
		enabledPushDevice("must-not-send", "com.labtether.mobile", "production"),
	}}
	adapter := &apnsFanoutAdapterStub{}
	deps := &Deps{
		PushDeviceStore: store,
		NotificationAdapters: map[string]notifications.Adapter{
			notifications.ChannelTypeAPNs: adapter,
		},
	}
	channel := notifications.Channel{Type: notifications.ChannelTypeAPNs, Config: map[string]any{
		"bundle_id": "com.labtether.mobile", "production": true,
	}}
	payload := map[string]any{
		"event": "alert.firing", "alert_id": "malformed-retry", "severity": "high",
		apnsRetryTargetsPayloadKey: "corrupt-target-list",
	}

	err := deps.sendNotification(context.Background(), channel, payload)
	if err == nil || !strings.Contains(err.Error(), "retry target restriction") {
		t.Fatalf("malformed retry restriction error = %v", err)
	}
	if len(adapter.calls) != 0 {
		t.Fatalf("malformed retry restriction expanded to %d APNs calls", len(adapter.calls))
	}
}

func TestSendAPNsNotificationRejectsUnconfiguredClientBundleTopic(t *testing.T) {
	device := enabledPushDevice("attacker-token", "com.example.other-team-app", "production")
	store := &apnsFanoutStoreStub{devices: []persistence.PushDevice{device}}
	adapter := &apnsFanoutAdapterStub{}
	deps := &Deps{
		PushDeviceStore: store,
		NotificationAdapters: map[string]notifications.Adapter{
			notifications.ChannelTypeAPNs: adapter,
		},
	}
	channel := notifications.Channel{
		Type: notifications.ChannelTypeAPNs,
		Config: map[string]any{
			"bundle_id":  "com.labtether.mobile",
			"production": true,
		},
	}

	if err := deps.sendNotification(context.Background(), channel, map[string]any{
		"event": "alert.firing", "alert_id": "alert-1", "severity": "high",
	}); err != nil {
		t.Fatalf("sendNotification: %v", err)
	}
	if len(adapter.calls) != 0 {
		t.Fatalf("unconfigured bundle topic received %d deliveries", len(adapter.calls))
	}
}

func TestPushDeviceAllowsPayloadHonorsCategorySeverityAndEventToggles(t *testing.T) {
	base := enabledPushDevice("token", "com.labtether.mobile", "production")
	tests := []struct {
		name    string
		mutate  func(*persistence.PushDevice)
		payload map[string]any
		want    bool
	}{
		{
			name: "severity below threshold",
			mutate: func(device *persistence.PushDevice) {
				device.MinimumSeverity = "critical"
			},
			payload: map[string]any{"event": "alert.firing", "alert_id": "a-1", "severity": "high"},
		},
		{
			name: "critical only rejects warning",
			mutate: func(device *persistence.PushDevice) {
				device.PushCategory = "critical_only"
			},
			payload: map[string]any{"event": "alert.firing", "alert_id": "a-1", "severity": "warning"},
		},
		{
			name:    "all alerts accepts warning",
			payload: map[string]any{"event": "alert.firing", "alert_id": "a-1", "severity": "warning"},
			want:    true,
		},
		{
			name:    "all alerts rejects incidents",
			payload: map[string]any{"event": "incident.opened", "incident_id": "i-1", "severity": "high"},
		},
		{
			name: "alerts and incidents accepts incidents",
			mutate: func(device *persistence.PushDevice) {
				device.PushCategory = "alerts_and_incidents"
			},
			payload: map[string]any{"event": "incident.opened", "incident_id": "i-1", "severity": "high"},
			want:    true,
		},
		{
			name: "generic critical toggle",
			mutate: func(device *persistence.PushDevice) {
				device.NotifyCriticalAlerts = false
			},
			payload: map[string]any{"event": "alert.firing", "alert_id": "a-1", "severity": "critical"},
		},
		{
			name: "node offline toggle",
			mutate: func(device *persistence.PushDevice) {
				device.NotifyNodeOffline = false
			},
			payload: map[string]any{
				"event": "alert.firing", "alert_id": "a-1", "severity": "critical", "rule_kind": "heartbeat_stale",
			},
		},
		{
			name: "node offline independent from generic critical toggle",
			mutate: func(device *persistence.PushDevice) {
				device.NotifyCriticalAlerts = false
			},
			payload: map[string]any{
				"event": "alert.firing", "alert_id": "a-1", "severity": "critical", "rule_kind": "heartbeat_stale",
			},
			want: true,
		},
		{
			name: "service down toggle",
			mutate: func(device *persistence.PushDevice) {
				device.NotifyServiceDown = false
			},
			payload: map[string]any{
				"event": "alert.firing", "alert_id": "a-1", "severity": "high",
				"labels": map[string]string{"signal": "down_transition_burst"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			device := base
			if tt.mutate != nil {
				tt.mutate(&device)
			}
			if got := pushDeviceAllowsPayload(device, tt.payload); got != tt.want {
				t.Fatalf("pushDeviceAllowsPayload() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPushDeviceAllowsPayloadHonorsQuietHoursInDeviceTimezone(t *testing.T) {
	device := enabledPushDevice("token", "com.labtether.mobile", "production")
	device.QuietHoursEnabled = true
	device.QuietHoursStartMinutes = 22 * 60
	device.QuietHoursEndMinutes = 7 * 60
	device.TimeZone = "Australia/Sydney"
	payload := map[string]any{"event": "alert.firing", "alert_id": "a-1", "severity": "high"}

	// January is AEDT (UTC+11), so 12:30 UTC is 23:30 in Sydney.
	quietNow := time.Date(2026, time.January, 15, 12, 30, 0, 0, time.UTC)
	if pushDeviceAllowsPayloadAt(device, payload, quietNow) {
		t.Fatal("non-critical alert should be suppressed during device-local quiet hours")
	}
	payload["severity"] = "critical"
	if !pushDeviceAllowsPayloadAt(device, payload, quietNow) {
		t.Fatal("critical alert should break through quiet hours")
	}

	payload["severity"] = "high"
	outsideNow := time.Date(2026, time.January, 15, 0, 30, 0, 0, time.UTC)
	if !pushDeviceAllowsPayloadAt(device, payload, outsideNow) {
		t.Fatal("alert should be delivered outside quiet hours")
	}
}

func TestAlertNotificationPayloadOnlyOffersActionsWhileFiring(t *testing.T) {
	rule := alerts.Rule{ID: "rule-1", Name: "CPU", Severity: "high"}
	firing := buildAlertNotificationPayload(rule, "instance-1", "firing", nil)
	if firing["apns_category"] != "LT_ALERT_ACTIONS" {
		t.Fatalf("firing payload category = %v", firing["apns_category"])
	}
	resolved := buildAlertNotificationPayload(rule, "instance-1", "resolved", nil)
	if _, present := resolved["apns_category"]; present {
		t.Fatalf("resolved payload must not expose stale mutation actions: %+v", resolved)
	}
}

func assertStringSliceEqual(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("slice = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("slice = %v, want %v", got, want)
		}
	}
}

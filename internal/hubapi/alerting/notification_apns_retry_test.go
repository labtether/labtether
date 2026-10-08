package alerting

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/labtether/labtether/internal/notifications"
	"github.com/labtether/labtether/internal/persistence"
	"strings"
	"testing"
	"time"
)

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

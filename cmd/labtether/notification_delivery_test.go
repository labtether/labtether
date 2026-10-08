package main

import (
	"context"
	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/groupmaintenance"
	"github.com/labtether/labtether/internal/groups"
	"github.com/labtether/labtether/internal/model"
	"github.com/labtether/labtether/internal/notifications"
	"strings"
	"testing"
	"time"
)

type fakeNotificationAdapter struct {
	typ     string
	sendErr error
	calls   []fakeNotificationCall
}

type fakeNotificationCall struct {
	Config  map[string]any
	Payload map[string]any
}

func (a *fakeNotificationAdapter) Type() string { return a.typ }

func (a *fakeNotificationAdapter) Send(_ context.Context, config map[string]any, payload map[string]any) error {
	a.calls = append(a.calls, fakeNotificationCall{
		Config:  cloneAnyMap(config),
		Payload: cloneAnyMap(payload),
	})
	return a.sendErr
}

type blockingNotificationAdapter struct {
	typ     string
	started chan struct{}
	release chan struct{}
}

func (a *blockingNotificationAdapter) Type() string { return a.typ }

func (a *blockingNotificationAdapter) Send(_ context.Context, _ map[string]any, _ map[string]any) error {
	select {
	case <-a.started:
	default:
		close(a.started)
	}
	<-a.release
	return nil
}

func TestDispatchAlertNotifications_SendsWebhookAndEmail(t *testing.T) {
	sut := newTestAPIServer(t)

	store := newNotificationStoreStub()
	store.channels["chan-webhook"] = notifications.Channel{
		ID:      "chan-webhook",
		Name:    "Webhook",
		Type:    notifications.ChannelTypeWebhook,
		Config:  map[string]any{"url": "https://example.invalid/hook"},
		Enabled: true,
	}
	store.channels["chan-email"] = notifications.Channel{
		ID:      "chan-email",
		Name:    "Email",
		Type:    notifications.ChannelTypeEmail,
		Config:  map[string]any{"smtp_host": "mail.example.invalid", "to": "ops@example.com"},
		Enabled: true,
	}
	store.routes["route-critical"] = notifications.Route{
		ID:             "route-critical",
		Name:           "Critical Route",
		Matchers:       map[string]any{"env": "lab"},
		ChannelIDs:     []string{"chan-webhook", "chan-email"},
		SeverityFilter: "critical",
		Enabled:        true,
	}

	webhookAdapter := &fakeNotificationAdapter{typ: notifications.ChannelTypeWebhook}
	emailAdapter := &fakeNotificationAdapter{typ: notifications.ChannelTypeEmail}
	sut.notificationStore = store
	sut.notificationDispatcher.Adapters = map[string]notifications.Adapter{
		notifications.ChannelTypeWebhook: webhookAdapter,
		notifications.ChannelTypeEmail:   emailAdapter,
	}

	rule := alerts.Rule{
		ID:       "rule-1",
		Name:     "CPU Saturation",
		Severity: alerts.SeverityCritical,
		Labels:   map[string]string{"env": "lab"},
		Targets:  []alerts.RuleTarget{{ID: "target-1", AssetID: "node-1"}},
	}

	sut.dispatchAlertNotifications(rule, "inst-1", "firing")
	sut.waitForNotificationDispatches()

	if len(webhookAdapter.calls) != 1 {
		t.Fatalf("expected one webhook call, got %d", len(webhookAdapter.calls))
	}
	if len(emailAdapter.calls) != 1 {
		t.Fatalf("expected one email call, got %d", len(emailAdapter.calls))
	}
	if got := webhookAdapter.calls[0].Payload["rule_id"]; got != "rule-1" {
		t.Fatalf("expected webhook payload rule_id=rule-1, got %v", got)
	}
	if got := emailAdapter.calls[0].Payload["to"]; got != "ops@example.com" {
		t.Fatalf("expected email payload to=ops@example.com, got %v", got)
	}

	if len(store.records) != 2 {
		t.Fatalf("expected two notification history records, got %d", len(store.records))
	}
	for _, record := range store.records {
		if record.Status != notifications.RecordStatusSent {
			t.Fatalf("expected sent status, got %+v", record)
		}
	}
}

func TestDispatchAlertNotifications_MaintenanceSuppressesDelivery(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.groupMaintenanceStore = &maintenanceOnlyGroupMaintenanceStore{}
	groupID := mustCreateGroup(t, sut, "Notification Maintenance", "notification-maintenance")
	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "node-1",
		GroupID: groupID,
		Status:  "online",
	}); err != nil {
		t.Fatalf("seed asset: %v", err)
	}
	if _, err := sut.groupMaintenanceStore.CreateGroupMaintenanceWindow(groupID, groupmaintenance.CreateMaintenanceWindowRequest{
		Name:           "Suppress alert delivery",
		StartAt:        time.Now().UTC().Add(-time.Minute),
		EndAt:          time.Now().UTC().Add(time.Minute),
		SuppressAlerts: true,
	}); err != nil {
		t.Fatalf("create maintenance window: %v", err)
	}

	store := newNotificationStoreStub()
	store.channels["chan-webhook"] = notifications.Channel{
		ID:      "chan-webhook",
		Name:    "Webhook",
		Type:    notifications.ChannelTypeWebhook,
		Enabled: true,
	}
	store.routes["route-all"] = notifications.Route{
		ID:         "route-all",
		Name:       "All alerts",
		ChannelIDs: []string{"chan-webhook"},
		Enabled:    true,
	}
	adapter := &fakeNotificationAdapter{typ: notifications.ChannelTypeWebhook}
	sut.notificationStore = store
	sut.notificationDispatcher.Adapters = map[string]notifications.Adapter{
		notifications.ChannelTypeWebhook: adapter,
	}
	rule := alerts.Rule{
		ID:       "rule-1",
		Name:     "Node offline",
		Severity: alerts.SeverityCritical,
		Targets:  []alerts.RuleTarget{{ID: "target-1", AssetID: "node-1"}},
	}

	sut.dispatchAlertNotifications(rule, "inst-1", "firing")
	sut.waitForNotificationDispatches()

	if len(adapter.calls) != 0 || len(store.records) != 0 {
		t.Fatalf("maintenance-suppressed delivery calls=%d records=%d", len(adapter.calls), len(store.records))
	}
}

func TestDispatchAlertNotifications_RecordsFailureWhenAdapterSendFails(t *testing.T) {
	sut := newTestAPIServer(t)

	store := newNotificationStoreStub()
	store.channels["chan-webhook"] = notifications.Channel{
		ID:      "chan-webhook",
		Name:    "Webhook",
		Type:    notifications.ChannelTypeWebhook,
		Config:  map[string]any{"url": "https://example.invalid/hook"},
		Enabled: true,
	}
	store.routes["route-critical"] = notifications.Route{
		ID:             "route-critical",
		Name:           "Critical Route",
		ChannelIDs:     []string{"chan-webhook"},
		SeverityFilter: "critical",
		Enabled:        true,
	}
	sut.notificationStore = store
	sut.notificationDispatcher.Adapters = map[string]notifications.Adapter{
		notifications.ChannelTypeWebhook: &fakeNotificationAdapter{
			typ:     notifications.ChannelTypeWebhook,
			sendErr: context.DeadlineExceeded,
		},
	}

	rule := alerts.Rule{
		ID:       "rule-1",
		Name:     "CPU Saturation",
		Severity: alerts.SeverityCritical,
	}
	sut.dispatchAlertNotifications(rule, "inst-1", "firing")
	sut.waitForNotificationDispatches()

	if len(store.records) != 1 {
		t.Fatalf("expected one notification history record, got %d", len(store.records))
	}
	if store.records[0].Status != notifications.RecordStatusFailed {
		t.Fatalf("expected failed status, got %s", store.records[0].Status)
	}
	if store.records[0].Error == "" {
		t.Fatalf("expected failure reason to be recorded")
	}
	if store.records[0].NextRetryAt == nil {
		t.Fatalf("expected failed record to schedule a retry")
	}
	if got := store.records[0].Payload["rule_id"]; got != "rule-1" {
		t.Fatalf("expected payload snapshot to retain rule_id, got %v", got)
	}
	if got := store.records[0].Payload["title"]; got == "" {
		t.Fatalf("expected payload snapshot to retain title, got %v", got)
	}
}

func TestDispatchAlertNotificationsDoesNotBlockCallerOnSlowChannel(t *testing.T) {
	sut := newTestAPIServer(t)

	store := newNotificationStoreStub()
	store.channels["chan-webhook"] = notifications.Channel{
		ID:      "chan-webhook",
		Name:    "Webhook",
		Type:    notifications.ChannelTypeWebhook,
		Config:  map[string]any{"url": "https://example.invalid/hook"},
		Enabled: true,
	}
	store.routes["route-critical"] = notifications.Route{
		ID:             "route-critical",
		Name:           "Critical Route",
		ChannelIDs:     []string{"chan-webhook"},
		SeverityFilter: "critical",
		Enabled:        true,
	}
	adapter := &blockingNotificationAdapter{
		typ:     notifications.ChannelTypeWebhook,
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	sut.notificationStore = store
	sut.notificationDispatcher.Adapters = map[string]notifications.Adapter{
		notifications.ChannelTypeWebhook: adapter,
	}

	rule := alerts.Rule{
		ID:       "rule-1",
		Name:     "CPU Saturation",
		Severity: alerts.SeverityCritical,
	}

	startedAt := time.Now()
	sut.dispatchAlertNotifications(rule, "inst-1", "firing")
	if elapsed := time.Since(startedAt); elapsed > 50*time.Millisecond {
		t.Fatalf("expected async dispatch to return quickly, took %s", elapsed)
	}

	select {
	case <-adapter.started:
	case <-time.After(time.Second):
		t.Fatal("expected background notification send to start")
	}

	close(adapter.release)
	sut.waitForNotificationDispatches()

	if len(store.records) != 1 {
		t.Fatalf("expected one notification history record after slow send, got %d", len(store.records))
	}
}

func TestRetryPendingNotifications_UsesStoredPayloadAndClearsFailureMetadataOnSuccess(t *testing.T) {
	sut := newTestAPIServer(t)

	store := newNotificationStoreStub()
	store.channels["chan-webhook"] = notifications.Channel{
		ID:      "chan-webhook",
		Name:    "Webhook",
		Type:    notifications.ChannelTypeWebhook,
		Config:  map[string]any{"url": "https://example.invalid/hook"},
		Enabled: true,
	}
	store.routes["route-critical"] = notifications.Route{
		ID:             "route-critical",
		Name:           "Critical Route",
		ChannelIDs:     []string{"chan-webhook"},
		SeverityFilter: "critical",
		Enabled:        true,
	}

	adapter := &fakeNotificationAdapter{
		typ:     notifications.ChannelTypeWebhook,
		sendErr: context.DeadlineExceeded,
	}
	sut.notificationStore = store
	sut.notificationDispatcher.Adapters = map[string]notifications.Adapter{
		notifications.ChannelTypeWebhook: adapter,
	}

	rule := alerts.Rule{
		ID:       "rule-1",
		Name:     "CPU Saturation",
		Severity: alerts.SeverityCritical,
	}
	sut.dispatchAlertNotifications(rule, "inst-1", "firing")
	sut.waitForNotificationDispatches()

	if len(store.records) != 1 {
		t.Fatalf("expected one failed notification record, got %d", len(store.records))
	}
	dueNow := time.Now().UTC().Add(-time.Second)
	store.records[0].NextRetryAt = &dueNow

	adapter.sendErr = nil
	sut.ensureAlertingDeps().RetryPendingNotifications(context.Background())

	if len(adapter.calls) != 2 {
		t.Fatalf("expected initial send plus retry, got %d calls", len(adapter.calls))
	}
	retryPayload := adapter.calls[1].Payload
	if got := retryPayload["rule_id"]; got != "rule-1" {
		t.Fatalf("expected retry to reuse original payload snapshot, got rule_id=%v", got)
	}
	if got := retryPayload["retry"]; got != true {
		t.Fatalf("expected retry marker on retry payload, got %v", got)
	}
	if got := retryPayload["title"]; got == "" {
		t.Fatalf("expected retry payload to retain title, got %v", got)
	}

	if store.records[0].Status != notifications.RecordStatusSent {
		t.Fatalf("expected retry success to mark record sent, got %s", store.records[0].Status)
	}
	if store.records[0].Error != "" {
		t.Fatalf("expected retry success to clear stale error, got %q", store.records[0].Error)
	}
	if store.records[0].SentAt == nil {
		t.Fatalf("expected retry success to set sent_at")
	}
	if store.records[0].NextRetryAt != nil {
		t.Fatalf("expected retry success to clear next_retry_at")
	}
}

func TestRouteMatchesAlert_CanonicalKindAndCapabilityMatchers(t *testing.T) {
	sut := newTestAPIServer(t)
	now := time.Now().UTC()
	group, err := sut.groupStore.CreateGroup(groups.CreateRequest{
		Name: "Group A",
		Slug: "group-a",
	})
	if err != nil {
		t.Fatalf("create group: %v", err)
	}

	_, err = sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "proxmox-vm-301",
		Type:    "vm",
		Name:    "vm-301",
		Source:  "proxmox",
		Status:  "online",
		GroupID: group.ID,
		Metadata: map[string]string{
			"resource_kind":  "vm",
			"resource_class": "compute",
		},
	})
	if err != nil {
		t.Fatalf("upsert heartbeat: %v", err)
	}

	_, err = sut.canonicalStore.UpsertCapabilitySet(model.CapabilitySet{
		SubjectType: "resource",
		SubjectID:   "proxmox-vm-301",
		Capabilities: []model.CapabilitySpec{
			{ID: "network.action", Scope: model.CapabilityScopeAction},
		},
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("upsert capability set: %v", err)
	}

	rule := alerts.Rule{
		ID:       "rule-canonical-route-match",
		Name:     "Route Predicate Test",
		Severity: alerts.SeverityHigh,
		Targets: []alerts.RuleTarget{
			{ID: "target-1", AssetID: "proxmox-vm-301"},
		},
	}
	predicateContext, _ := sut.buildAlertPredicateContext(rule, nil)

	route := notifications.Route{
		ID:          "route-canonical",
		Name:        "Canonical Route",
		Enabled:     true,
		GroupFilter: group.ID,
		Matchers: map[string]any{
			"resource_kind":  "vm",
			"resource_class": "compute",
			"capability":     "network.action",
		},
	}

	if !sut.routeMatchesAlert(route, rule, "firing", predicateContext) {
		t.Fatalf("expected canonical matcher combination to match")
	}

	route.Matchers["capabilities_all"] = []any{"network.action", "missing.capability"}
	if sut.routeMatchesAlert(route, rule, "firing", predicateContext) {
		t.Fatalf("expected route to fail when capabilities_all includes missing capability")
	}
}

func TestValidateCreateRouteRequest_RejectsDeprecatedMatcherKeys(t *testing.T) {
	err := validateCreateRouteRequest(notifications.CreateRouteRequest{
		Name:     "Legacy matcher route",
		Matchers: map[string]any{"target_kind": "vm"},
	})
	if err == nil {
		t.Fatalf("expected deprecated matcher validation error")
	}
	if !strings.Contains(err.Error(), "deprecated") {
		t.Fatalf("expected deprecated matcher key error, got %v", err)
	}
}

func TestValidateUpdateRouteRequest_RejectsDeprecatedMatcherKeys(t *testing.T) {
	matchers := map[string]any{"target_class": "compute"}
	err := validateUpdateRouteRequest(notifications.UpdateRouteRequest{
		Matchers: &matchers,
	})
	if err == nil {
		t.Fatalf("expected deprecated matcher validation error")
	}
	if !strings.Contains(err.Error(), "deprecated") {
		t.Fatalf("expected deprecated matcher key error, got %v", err)
	}
}

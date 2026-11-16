package main

import (
	"context"
	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/groupmaintenance"
	"github.com/labtether/labtether/internal/notifications"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/hubapi/shared"
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
		Config:  shared.CloneAnyMap(config),
		Payload: shared.CloneAnyMap(payload),
	})
	return a.sendErr
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

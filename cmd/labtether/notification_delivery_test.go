package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/groups"
	"github.com/labtether/labtether/internal/model"
	"github.com/labtether/labtether/internal/notifications"
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

type notificationStoreStub struct {
	channels      map[string]notifications.Channel
	routes        map[string]notifications.Route
	records       []notifications.Record
	retryUpdates  []notificationRetryUpdate
	nextRecordNum int
}

type notificationRetryUpdate struct {
	ID          string
	RetryCount  int
	NextRetryAt *time.Time
	Status      string
	Error       string
}

func newNotificationStoreStub() *notificationStoreStub {
	return &notificationStoreStub{
		channels:      make(map[string]notifications.Channel),
		routes:        make(map[string]notifications.Route),
		records:       make([]notifications.Record, 0, 8),
		retryUpdates:  make([]notificationRetryUpdate, 0, 8),
		nextRecordNum: 1,
	}
}

func (s *notificationStoreStub) CreateNotificationChannel(req notifications.CreateChannelRequest) (notifications.Channel, error) {
	id := "nch-" + req.Name
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	ch := notifications.Channel{
		ID:        id,
		Name:      req.Name,
		Type:      req.Type,
		Config:    cloneAnyMap(req.Config),
		Enabled:   enabled,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	s.channels[id] = ch
	return ch, nil
}

func (s *notificationStoreStub) GetNotificationChannel(id string) (notifications.Channel, bool, error) {
	ch, ok := s.channels[id]
	return ch, ok, nil
}

func (s *notificationStoreStub) ListNotificationChannels(_ int) ([]notifications.Channel, error) {
	out := make([]notifications.Channel, 0, len(s.channels))
	for _, ch := range s.channels {
		out = append(out, ch)
	}
	return out, nil
}

func (s *notificationStoreStub) UpdateNotificationChannel(id string, req notifications.UpdateChannelRequest) (notifications.Channel, error) {
	ch, ok := s.channels[id]
	if !ok {
		return notifications.Channel{}, notifications.ErrChannelNotFound
	}
	if req.Name != nil {
		ch.Name = *req.Name
	}
	if req.Config != nil {
		ch.Config = cloneAnyMap(*req.Config)
	}
	if req.Enabled != nil {
		ch.Enabled = *req.Enabled
	}
	ch.UpdatedAt = time.Now().UTC()
	s.channels[id] = ch
	return ch, nil
}

func (s *notificationStoreStub) DeleteNotificationChannel(id string) error {
	if _, ok := s.channels[id]; !ok {
		return notifications.ErrChannelNotFound
	}
	delete(s.channels, id)
	return nil
}

func (s *notificationStoreStub) CreateAlertRoute(req notifications.CreateRouteRequest) (notifications.Route, error) {
	id := "route-" + req.Name
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	route := notifications.Route{
		ID:                    id,
		Name:                  req.Name,
		Matchers:              cloneAnyMap(req.Matchers),
		ChannelIDs:            append([]string(nil), req.ChannelIDs...),
		SeverityFilter:        req.SeverityFilter,
		GroupFilter:           req.GroupFilter,
		GroupBy:               append([]string(nil), req.GroupBy...),
		GroupWaitSeconds:      req.GroupWaitSeconds,
		GroupIntervalSeconds:  req.GroupIntervalSeconds,
		RepeatIntervalSeconds: req.RepeatIntervalSeconds,
		Enabled:               enabled,
		CreatedAt:             time.Now().UTC(),
		UpdatedAt:             time.Now().UTC(),
	}
	s.routes[id] = route
	return route, nil
}

func (s *notificationStoreStub) GetAlertRoute(id string) (notifications.Route, bool, error) {
	route, ok := s.routes[id]
	return route, ok, nil
}

func (s *notificationStoreStub) ListAlertRoutes(_ int) ([]notifications.Route, error) {
	out := make([]notifications.Route, 0, len(s.routes))
	for _, route := range s.routes {
		out = append(out, route)
	}
	return out, nil
}

func (s *notificationStoreStub) UpdateAlertRoute(id string, req notifications.UpdateRouteRequest) (notifications.Route, error) {
	route, ok := s.routes[id]
	if !ok {
		return notifications.Route{}, notifications.ErrRouteNotFound
	}
	if req.Name != nil {
		route.Name = *req.Name
	}
	if req.Matchers != nil {
		route.Matchers = cloneAnyMap(*req.Matchers)
	}
	if req.ChannelIDs != nil {
		route.ChannelIDs = append([]string(nil), (*req.ChannelIDs)...)
	}
	if req.SeverityFilter != nil {
		route.SeverityFilter = *req.SeverityFilter
	}
	if req.GroupFilter != nil {
		route.GroupFilter = *req.GroupFilter
	}
	if req.GroupBy != nil {
		route.GroupBy = append([]string(nil), (*req.GroupBy)...)
	}
	if req.GroupWaitSeconds != nil {
		route.GroupWaitSeconds = *req.GroupWaitSeconds
	}
	if req.GroupIntervalSeconds != nil {
		route.GroupIntervalSeconds = *req.GroupIntervalSeconds
	}
	if req.RepeatIntervalSeconds != nil {
		route.RepeatIntervalSeconds = *req.RepeatIntervalSeconds
	}
	if req.Enabled != nil {
		route.Enabled = *req.Enabled
	}
	route.UpdatedAt = time.Now().UTC()
	s.routes[id] = route
	return route, nil
}

func (s *notificationStoreStub) DeleteAlertRoute(id string) error {
	if _, ok := s.routes[id]; !ok {
		return notifications.ErrRouteNotFound
	}
	delete(s.routes, id)
	return nil
}

func (s *notificationStoreStub) CreateNotificationRecord(req notifications.CreateRecordRequest) (notifications.Record, error) {
	now := time.Now().UTC()
	rec := notifications.Record{
		ID:              "notif-record-" + time.Now().UTC().Format("150405.000000000"),
		ChannelID:       req.ChannelID,
		AlertInstanceID: req.AlertInstanceID,
		RouteID:         req.RouteID,
		Payload:         cloneAnyMap(req.Payload),
		Status:          req.Status,
		Error:           req.Error,
		MaxRetries:      notifications.DefaultMaxRetries,
		CreatedAt:       now,
	}
	if rec.Status == notifications.RecordStatusSent {
		rec.SentAt = &now
	}
	s.nextRecordNum++
	s.records = append(s.records, rec)
	return rec, nil
}

func (s *notificationStoreStub) ListNotificationHistory(_ int, channelID string) ([]notifications.Record, error) {
	out := make([]notifications.Record, 0, len(s.records))
	for _, rec := range s.records {
		if channelID != "" && rec.ChannelID != channelID {
			continue
		}
		out = append(out, notifications.Record{
			ID:              rec.ID,
			ChannelID:       rec.ChannelID,
			AlertInstanceID: rec.AlertInstanceID,
			RouteID:         rec.RouteID,
			Payload:         cloneAnyMap(rec.Payload),
			Status:          rec.Status,
			SentAt:          rec.SentAt,
			Error:           rec.Error,
			RetryCount:      rec.RetryCount,
			MaxRetries:      rec.MaxRetries,
			NextRetryAt:     rec.NextRetryAt,
			CreatedAt:       rec.CreatedAt,
		})
	}
	return out, nil
}

func (s *notificationStoreStub) ListPendingRetries(_ context.Context, now time.Time, limit int) ([]notifications.Record, error) {
	if limit <= 0 {
		limit = 50
	}
	out := make([]notifications.Record, 0, limit)
	for _, rec := range s.records {
		if rec.Status != notifications.RecordStatusFailed || rec.NextRetryAt == nil {
			continue
		}
		if rec.RetryCount >= rec.MaxRetries {
			continue
		}
		if rec.NextRetryAt.After(now) {
			continue
		}
		out = append(out, notifications.Record{
			ID:              rec.ID,
			ChannelID:       rec.ChannelID,
			AlertInstanceID: rec.AlertInstanceID,
			RouteID:         rec.RouteID,
			Payload:         cloneAnyMap(rec.Payload),
			Status:          rec.Status,
			SentAt:          rec.SentAt,
			Error:           rec.Error,
			RetryCount:      rec.RetryCount,
			MaxRetries:      rec.MaxRetries,
			NextRetryAt:     rec.NextRetryAt,
			CreatedAt:       rec.CreatedAt,
		})
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (s *notificationStoreStub) UpdateRetryState(_ context.Context, id string, retryCount int, nextRetryAt *time.Time, status, errorMessage string) error {
	s.retryUpdates = append(s.retryUpdates, notificationRetryUpdate{
		ID:          id,
		RetryCount:  retryCount,
		NextRetryAt: nextRetryAt,
		Status:      status,
		Error:       errorMessage,
	})
	for idx := range s.records {
		if s.records[idx].ID != id {
			continue
		}
		s.records[idx].RetryCount = retryCount
		s.records[idx].NextRetryAt = nextRetryAt
		s.records[idx].Status = status
		s.records[idx].Error = errorMessage
		if status == notifications.RecordStatusSent {
			sentAt := time.Now().UTC()
			s.records[idx].SentAt = &sentAt
			s.records[idx].Error = ""
		} else {
			s.records[idx].SentAt = nil
		}
		break
	}
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

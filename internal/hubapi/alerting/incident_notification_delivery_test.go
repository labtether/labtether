package alerting

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/groups"
	"github.com/labtether/labtether/internal/hubapi/groupfeatures"
	"github.com/labtether/labtether/internal/incidents"
	"github.com/labtether/labtether/internal/notifications"
	"github.com/labtether/labtether/internal/persistence"
)

type incidentPushDeviceStoreStub struct {
	devices []persistence.PushDevice
}

func TestIncidentDispatchPublishesWebhookEventWithoutNotificationRuntime(t *testing.T) {
	var eventType string
	var eventData map[string]any
	deps := &Deps{
		Broadcast: func(gotType string, gotData map[string]any) {
			eventType = gotType
			eventData = gotData
		},
	}
	deps.dispatchIncidentNotificationAsync(incidents.Incident{
		ID:       "incident-webhook",
		Status:   incidents.StatusOpen,
		Severity: incidents.SeverityHigh,
	}, "incident.created")

	if eventType != "incident.created" {
		t.Fatalf("event type = %q, want incident.created", eventType)
	}
	if eventData["incident_id"] != "incident-webhook" {
		t.Fatalf("event data = %#v, want incident id", eventData)
	}
}

func (s *incidentPushDeviceStoreStub) GetAllPushTokens(context.Context) ([]persistence.PushDevice, error) {
	return append([]persistence.PushDevice(nil), s.devices...), nil
}

func (s *incidentPushDeviceStoreStub) DeletePushDeviceByToken(context.Context, string, string, string) error {
	return nil
}

type incidentNotificationAdapterCall struct {
	config  map[string]any
	payload map[string]any
}

type incidentNotificationAdapterStub struct {
	typ     string
	started chan struct{}
	release <-chan struct{}
	once    sync.Once
	mu      sync.Mutex
	calls   []incidentNotificationAdapterCall
}

func (a *incidentNotificationAdapterStub) Type() string { return a.typ }

func (a *incidentNotificationAdapterStub) Send(ctx context.Context, config map[string]any, payload map[string]any) error {
	a.mu.Lock()
	a.calls = append(a.calls, incidentNotificationAdapterCall{
		config:  cloneAnyMap(config),
		payload: cloneAnyMap(payload),
	})
	a.mu.Unlock()
	if a.started != nil {
		a.once.Do(func() { close(a.started) })
	}
	if a.release != nil {
		select {
		case <-a.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (a *incidentNotificationAdapterStub) snapshot() []incidentNotificationAdapterCall {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]incidentNotificationAdapterCall(nil), a.calls...)
}

func (a *incidentNotificationAdapterStub) reset() {
	a.mu.Lock()
	a.calls = nil
	a.mu.Unlock()
}

func incidentPushDevice(token, category, minimumSeverity string) persistence.PushDevice {
	return persistence.PushDevice{
		ID:                     "device-" + token,
		Platform:               "ios",
		PushToken:              token,
		BundleID:               "com.labtether.mobile",
		Environment:            "production",
		TimeZone:               "UTC",
		NotifyCriticalAlerts:   true,
		NotifyNodeOffline:      true,
		NotifyServiceDown:      true,
		PushCategory:           category,
		MinimumSeverity:        minimumSeverity,
		QuietHoursStartMinutes: 22 * 60,
		QuietHoursEndMinutes:   7 * 60,
	}
}

func configureIncidentNotificationTest(
	t *testing.T,
	deps *Deps,
	adapter notifications.Adapter,
	devices []persistence.PushDevice,
) *notificationSecurityStore {
	t.Helper()
	store := newNotificationSecurityStore()
	store.seed(notifications.Channel{
		ID:      "incident-apns",
		Name:    "Incident push",
		Type:    notifications.ChannelTypeAPNs,
		Enabled: true,
		Config: map[string]any{
			"bundle_id":  "com.labtether.mobile",
			"production": true,
		},
	})
	deps.NotificationStore = store
	deps.PushDeviceStore = &incidentPushDeviceStoreStub{devices: devices}
	deps.NotificationAdapters = map[string]notifications.Adapter{
		notifications.ChannelTypeAPNs: adapter,
	}
	return store
}

func TestIncidentCreationQueuesBoundedAPNsDeliveryWithoutBlockingHTTP(t *testing.T) {
	deps := newTestAlertingDeps(t)
	started := make(chan struct{})
	release := make(chan struct{})
	adapter := &incidentNotificationAdapterStub{
		typ:     notifications.ChannelTypeAPNs,
		started: started,
		release: release,
	}
	store := configureIncidentNotificationTest(t, deps, adapter, []persistence.PushDevice{
		incidentPushDevice("incident-token", "alerts_and_incidents", "low"),
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/incidents",
		bytes.NewReader([]byte(`{"title":"Database unavailable","summary":"Primary storage is unreachable.","severity":"critical"}`)),
	)
	returned := make(chan struct{})
	go func() {
		deps.HandleIncidents(recorder, request)
		close(returned)
	}()

	select {
	case <-returned:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("incident create handler blocked on APNs network delivery")
	}
	if recorder.Code != http.StatusCreated {
		close(release)
		t.Fatalf("create status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("incident APNs delivery was not started")
	}
	close(release)
	deps.WaitForNotificationDispatches()

	var response struct {
		Incident incidents.Incident `json:"incident"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode incident response: %v", err)
	}
	calls := adapter.snapshot()
	if len(calls) != 1 {
		t.Fatalf("APNs calls = %d, want 1", len(calls))
	}
	payload := calls[0].payload
	assertIncidentPushField(t, payload, "incident_id", response.Incident.ID)
	assertIncidentPushField(t, payload, "event", "incident.created")
	assertIncidentPushField(t, payload, "severity", "critical")
	assertIncidentPushField(t, payload, "status", "open")
	assertIncidentPushField(t, payload, "title", "Database unavailable")
	assertIncidentPushField(t, payload, "summary", "Primary storage is unreachable.")
	assertIncidentPushField(t, payload, "deep_link", "labtether://incidents/"+response.Incident.ID)
	assertIncidentPushField(t, payload, "apns_category", "LT_INCIDENT_ACTIONS")
	if tokens, _ := calls[0].config["device_tokens"].([]string); len(tokens) != 1 || tokens[0] != "incident-token" {
		t.Fatalf("APNs device tokens = %v, want incident-token", tokens)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.records) != 1 ||
		store.records[0].Status != notifications.RecordStatusSent ||
		store.records[0].AlertInstanceID != "" ||
		store.records[0].RouteID != "" ||
		payloadString(store.records[0].Payload, "incident_id") != response.Incident.ID {
		t.Fatalf("incident notification history = %+v", store.records)
	}
}

func TestIncidentUpdatesNotifyOnlyMaterialStatusOrSeverityTransitions(t *testing.T) {
	deps := newTestAlertingDeps(t)
	adapter := &incidentNotificationAdapterStub{typ: notifications.ChannelTypeAPNs}
	configureIncidentNotificationTest(t, deps, adapter, []persistence.PushDevice{
		incidentPushDevice("transition-token", "alerts_and_incidents", "low"),
	})
	incident, err := deps.IncidentStore.CreateIncident(incidents.CreateIncidentRequest{
		Title:    "Transition test",
		Summary:  "Initial summary",
		Severity: incidents.SeverityMedium,
	})
	if err != nil {
		t.Fatalf("create incident fixture: %v", err)
	}

	patchIncidentForPushTest(t, deps, incident.ID, `{"summary":"Edited without material transition"}`)
	deps.WaitForNotificationDispatches()
	if calls := adapter.snapshot(); len(calls) != 0 {
		t.Fatalf("summary-only update sent %d pushes", len(calls))
	}

	patchIncidentForPushTest(t, deps, incident.ID, `{"status":"open","severity":"medium"}`)
	deps.WaitForNotificationDispatches()
	if calls := adapter.snapshot(); len(calls) != 0 {
		t.Fatalf("no-op status/severity update sent %d pushes", len(calls))
	}

	patchIncidentForPushTest(t, deps, incident.ID, `{"status":"investigating"}`)
	deps.WaitForNotificationDispatches()
	calls := adapter.snapshot()
	if len(calls) != 1 {
		t.Fatalf("status transition pushes = %d, want 1", len(calls))
	}
	assertIncidentPushField(t, calls[0].payload, "event", "incident.status_changed")
	assertIncidentPushField(t, calls[0].payload, "status", "investigating")
	adapter.reset()

	patchIncidentForPushTest(t, deps, incident.ID, `{"severity":"high"}`)
	deps.WaitForNotificationDispatches()
	calls = adapter.snapshot()
	if len(calls) != 1 {
		t.Fatalf("severity transition pushes = %d, want 1", len(calls))
	}
	assertIncidentPushField(t, calls[0].payload, "event", "incident.severity_changed")
	assertIncidentPushField(t, calls[0].payload, "severity", "high")
	adapter.reset()

	patchIncidentForPushTest(t, deps, incident.ID, `{"status":"resolved","severity":"critical"}`)
	deps.WaitForNotificationDispatches()
	calls = adapter.snapshot()
	if len(calls) != 1 {
		t.Fatalf("combined material transition pushes = %d, want one de-duplicated push", len(calls))
	}
	assertIncidentPushField(t, calls[0].payload, "event", "incident.resolved")
	assertIncidentPushField(t, calls[0].payload, "status", "resolved")
	assertIncidentPushField(t, calls[0].payload, "severity", "critical")
}

func TestIncidentDeliveryUsesOnlyEnabledAPNsChannelsAndEligibleDevices(t *testing.T) {
	deps := newTestAlertingDeps(t)
	apnsAdapter := &incidentNotificationAdapterStub{typ: notifications.ChannelTypeAPNs}
	webhookAdapter := &incidentNotificationAdapterStub{typ: notifications.ChannelTypeWebhook}
	store := configureIncidentNotificationTest(t, deps, apnsAdapter, []persistence.PushDevice{
		incidentPushDevice("eligible", "alerts_and_incidents", "low"),
		incidentPushDevice("alerts-only", "all_alerts", "low"),
		incidentPushDevice("critical-only", "critical_only", "low"),
		incidentPushDevice("severity-too-low", "alerts_and_incidents", "critical"),
	})
	store.seed(notifications.Channel{
		ID:      "disabled-apns",
		Type:    notifications.ChannelTypeAPNs,
		Enabled: false,
		Config:  map[string]any{"bundle_id": "com.labtether.mobile", "production": true},
	})
	store.seed(notifications.Channel{
		ID:      "enabled-webhook",
		Type:    notifications.ChannelTypeWebhook,
		Enabled: true,
		Config:  map[string]any{"url": "https://example.invalid/incidents"},
	})
	deps.NotificationAdapters[notifications.ChannelTypeWebhook] = webhookAdapter

	deps.dispatchIncidentNotificationAsync(incidents.Incident{
		ID:       "incident-filtering",
		Title:    "Filtering test",
		Status:   incidents.StatusOpen,
		Severity: incidents.SeverityHigh,
	}, "incident.created")
	deps.WaitForNotificationDispatches()

	if calls := webhookAdapter.snapshot(); len(calls) != 0 {
		t.Fatalf("incident delivery reached non-APNs channel %d times", len(calls))
	}
	calls := apnsAdapter.snapshot()
	if len(calls) != 1 {
		t.Fatalf("enabled APNs calls = %d, want 1", len(calls))
	}
	tokens, _ := calls[0].config["device_tokens"].([]string)
	if len(tokens) != 1 || tokens[0] != "eligible" {
		t.Fatalf("eligible incident device tokens = %v, want [eligible]", tokens)
	}

	quietDevice := incidentPushDevice("quiet", "alerts_and_incidents", "low")
	quietDevice.QuietHoursEnabled = true
	quietDevice.TimeZone = "Australia/Sydney"
	quietTime := time.Date(2026, time.January, 15, 12, 30, 0, 0, time.UTC) // 23:30 AEDT
	payload := buildIncidentNotificationPayload(incidents.Incident{
		ID: "quiet-incident", Title: "Quiet", Status: incidents.StatusOpen, Severity: incidents.SeverityHigh,
	}, "incident.created")
	if pushDeviceAllowsPayloadAt(quietDevice, payload, quietTime) {
		t.Fatal("non-critical incident bypassed device-local quiet hours")
	}
	payload["severity"] = incidents.SeverityCritical
	if !pushDeviceAllowsPayloadAt(quietDevice, payload, quietTime) {
		t.Fatal("critical incident did not break through quiet hours")
	}
}

func TestIncidentGroupMaintenanceSuppressesCreationPush(t *testing.T) {
	deps := newTestAlertingDeps(t)
	adapter := &incidentNotificationAdapterStub{typ: notifications.ChannelTypeAPNs}
	store := configureIncidentNotificationTest(t, deps, adapter, []persistence.PushDevice{
		incidentPushDevice("maintenance-token", "alerts_and_incidents", "low"),
	})
	group, err := deps.GroupStore.CreateGroup(groups.CreateRequest{Name: "Maintenance group"})
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	deps.EvaluateGuardrails = func(groupID string, _ time.Time) (groupfeatures.GroupMaintenanceGuardrails, error) {
		return groupfeatures.GroupMaintenanceGuardrails{GroupID: groupID, SuppressAlerts: groupID == group.ID}, nil
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/incidents",
		bytes.NewReader([]byte(`{"title":"Planned maintenance","severity":"high","group_id":"`+group.ID+`"}`)),
	)
	deps.HandleIncidents(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("create status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	deps.WaitForNotificationDispatches()
	if calls := adapter.snapshot(); len(calls) != 0 {
		t.Fatalf("maintenance-suppressed incident sent %d pushes", len(calls))
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.records) != 0 {
		t.Fatalf("maintenance-suppressed incident persisted delivery history: %+v", store.records)
	}
}

func TestBuildIncidentNotificationPayloadBoundsUnicodeFields(t *testing.T) {
	payload := buildIncidentNotificationPayload(incidents.Incident{
		ID:       "incident-bounds",
		Title:    string(bytes.Repeat([]byte("🧪"), 100)),
		Summary:  string(bytes.Repeat([]byte("summary"), 500)),
		Status:   incidents.StatusInvestigating,
		Severity: incidents.SeverityCritical,
	}, "incident.status_changed")
	if len(payloadString(payload, "title")) > incidentPushTitleMaxBytes {
		t.Fatalf("title bytes = %d, want <= %d", len(payloadString(payload, "title")), incidentPushTitleMaxBytes)
	}
	if len(payloadString(payload, "summary")) > incidentPushSummaryMaxBytes {
		t.Fatalf("summary bytes = %d, want <= %d", len(payloadString(payload, "summary")), incidentPushSummaryMaxBytes)
	}
	if !json.Valid([]byte(`"` + payloadString(payload, "title") + `"`)) {
		t.Fatal("bounded title is not valid UTF-8 JSON text")
	}
}

func patchIncidentForPushTest(t *testing.T, deps *Deps, incidentID, body string) {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPatch, "/incidents/"+incidentID, bytes.NewBufferString(body))
	deps.HandleIncidentActions(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("patch incident status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func assertIncidentPushField(t *testing.T, payload map[string]any, key, want string) {
	t.Helper()
	if got := payloadString(payload, key); got != want {
		t.Fatalf("incident push %s = %q, want %q", key, got, want)
	}
}

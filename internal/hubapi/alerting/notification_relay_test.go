package alerting

// Risks covered: a push sent across transports (relay id to Apple, raw token
// to the relay) is rejected and the cleanup handler then deletes a valid
// registration; a relay "gone" answer must delete only that registration;
// the default relay channel must be created once and never fight operator
// edits; a lowercased relay id breaks every Live Activity push.

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/labtether/labtether/internal/notifications"
	"github.com/labtether/labtether/internal/persistence"
)

const (
	relayTestIDKept = "KeptRelayRegistration_0123456789abcdefghijk"
	relayTestIDGone = "GoneRelayRegistration_0123456789ABCDEFGHIJK"
)

func relayPushDevice(id string) persistence.PushDevice {
	device := enabledPushDevice(id, "", "production")
	device.ID = "device-" + id[:4]
	device.PushTransport = notifications.APNsTransportRelay
	return device
}

func TestAPNsChannelsSendOnlyTheirOwnTransport(t *testing.T) {
	store := &apnsFanoutStoreStub{devices: []persistence.PushDevice{
		enabledPushDevice("raw-direct-token", notifications.OfficialAppBundleID, "production"),
		relayPushDevice(relayTestIDKept),
	}}
	adapter := &apnsFanoutAdapterStub{}
	deps := &Deps{PushDeviceStore: store, NotificationAdapters: map[string]notifications.Adapter{
		notifications.ChannelTypeAPNs: adapter,
	}}
	payload := map[string]any{"event": "alert.firing", "alert_id": "a-1", "severity": "high"}
	relay := notifications.Channel{Type: notifications.ChannelTypeAPNs, Config: map[string]any{"transport": "relay"}}
	direct := notifications.Channel{Type: notifications.ChannelTypeAPNs, Config: map[string]any{
		"bundle_id": notifications.OfficialAppBundleID, "production": true,
	}}
	for _, channel := range []notifications.Channel{relay, direct} {
		if err := deps.sendNotification(context.Background(), channel, payload); err != nil {
			t.Fatalf("send: %v", err)
		}
	}
	if len(adapter.calls) != 2 {
		t.Fatalf("adapter calls = %d, want one per channel", len(adapter.calls))
	}
	for index, want := range []struct{ token, transport string }{
		{relayTestIDKept, notifications.APNsTransportRelay},
		{"raw-direct-token", notifications.APNsTransportDirect},
	} {
		config := adapter.calls[index].config
		tokens, _ := config["device_tokens"].([]string)
		if len(tokens) != 1 || tokens[0] != want.token || config["transport"] != want.transport {
			t.Fatalf("call %d sent %v over %v, want only %s over %s", index, tokens, config["transport"], want.token, want.transport)
		}
	}
}

func TestRelayGoneAnswerDeletesOnlyThatRelayRegistration(t *testing.T) {
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		w.Header().Set("Content-Type", "application/json")
		if body["registration_id"] == relayTestIDGone {
			w.WriteHeader(http.StatusGone)
			_, _ = w.Write([]byte(`{"error":"unregistered"}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(server.Close)
	store := &apnsFanoutStoreStub{devices: []persistence.PushDevice{
		relayPushDevice(relayTestIDKept),
		relayPushDevice(relayTestIDGone),
		enabledPushDevice("raw-direct-token", notifications.OfficialAppBundleID, "production"),
	}}
	deps := &Deps{PushDeviceStore: store, NotificationAdapters: map[string]notifications.Adapter{
		notifications.ChannelTypeAPNs: &notifications.APNsAdapter{Relay: notifications.NewAPNsRelayClient(server.URL)},
	}}
	channel := notifications.Channel{Type: notifications.ChannelTypeAPNs, Config: map[string]any{"transport": "relay"}}
	err := deps.sendNotification(context.Background(), channel, map[string]any{
		"event": "alert.firing", "alert_id": "a-1", "severity": "high",
	})
	if err == nil {
		t.Fatal("a gone registration must be reported as a failed delivery")
	}
	if strings.Contains(err.Error(), relayTestIDGone) || strings.Contains(err.Error(), relayTestIDKept) {
		t.Fatal("relay registration id reached the delivery error")
	}
	want := [3]string{relayTestIDGone, notifications.OfficialAppBundleID, "production"}
	if len(store.deleted) != 1 || store.deleted[0] != want || store.deletedTransports[0] != notifications.APNsTransportRelay {
		t.Fatalf("cleanup = %v %v, want only the gone relay registration", store.deleted, store.deletedTransports)
	}
}

type relayDefaultStore struct {
	*notificationSecurityStore
	routes []notifications.Route
}

func (s *relayDefaultStore) CreateAlertRoute(req notifications.CreateRouteRequest) (notifications.Route, error) {
	route := notifications.Route{ID: "route-" + req.Name, Name: req.Name, ChannelIDs: req.ChannelIDs, Enabled: true}
	s.routes = append(s.routes, route)
	return route, nil
}

func (s *relayDefaultStore) ListAlertRoutes(int) ([]notifications.Route, error) {
	return append([]notifications.Route(nil), s.routes...), nil
}

type bootstrapMarkerStub struct {
	mu      sync.Mutex
	claimed map[string]bool
}

func (s *bootstrapMarkerStub) ClaimNotificationBootstrap(_ context.Context, name string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.claimed[name] {
		return false, nil
	}
	s.claimed[name] = true
	return true, nil
}

func (s *bootstrapMarkerStub) ReleaseNotificationBootstrap(_ context.Context, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.claimed, name)
	return nil
}

func TestDefaultRelayPushRouteIsCreatedOnceAndRespectsOperatorChoices(t *testing.T) {
	store := &relayDefaultStore{notificationSecurityStore: newNotificationSecurityStore()}
	markers := &bootstrapMarkerStub{claimed: map[string]bool{}}
	deps := &Deps{NotificationStore: store, NotificationAdapters: map[string]notifications.Adapter{
		notifications.ChannelTypeAPNs: &notifications.APNsAdapter{Relay: notifications.NewAPNsRelayClient(notifications.DefaultPushRelayURL)},
	}}
	for attempt := 0; attempt < 3; attempt++ {
		created, err := deps.EnsureDefaultRelayPushRoute(context.Background(), markers)
		if err != nil || created != (attempt == 0) {
			t.Fatalf("attempt %d: created=%t err=%v", attempt, created, err)
		}
	}
	channels, _ := store.ListNotificationChannels(10, 0)
	if len(channels) != 1 || len(store.routes) != 1 || store.routes[0].ChannelIDs[0] != channels[0].ID {
		t.Fatalf("want one relay channel and one route to it, got %d channels, routes %+v", len(channels), store.routes)
	}
	if channels[0].Config["transport"] != "relay" || !channels[0].Enabled {
		t.Fatalf("default channel is not an enabled relay channel: %+v", channels[0])
	}

	// An operator who removed the default keeps that choice.
	_ = store.DeleteNotificationChannel(channels[0].ID)
	store.routes = nil
	if created, err := deps.EnsureDefaultRelayPushRoute(context.Background(), markers); created || err != nil {
		t.Fatalf("default recreated after operator deletion: created=%t err=%v", created, err)
	}

	// A Hub that already routes to its own APNs channel gets nothing automatic.
	fresh := &relayDefaultStore{notificationSecurityStore: newNotificationSecurityStore()}
	fresh.seed(notifications.Channel{ID: "own-key", Type: notifications.ChannelTypeAPNs, Enabled: true})
	fresh.routes = []notifications.Route{{ID: "r1", ChannelIDs: []string{"own-key"}}}
	deps.NotificationStore = fresh
	if created, err := deps.EnsureDefaultRelayPushRoute(context.Background(), &bootstrapMarkerStub{claimed: map[string]bool{}}); created || err != nil {
		t.Fatalf("default created beside an operator APNs route: created=%t err=%v", created, err)
	}
}

func TestLiveActivityRelayRegistrationKeepsIDCase(t *testing.T) {
	request := registerLiveActivityRequest{DeviceID: "device-1", RelayRegistrationID: " " + relayTestIDGone + " ", Environment: "Production"}
	normalizeLiveActivityRegistration(&request)
	if message := validateLiveActivityRegistration(request); message != "" {
		t.Fatalf("valid relay registration refused: %s", message)
	}
	if credential, transport := request.credential(); credential != relayTestIDGone || transport != notifications.APNsTransportRelay {
		t.Fatalf("relay credential changed: transport %s", transport)
	}
	request.PushToken = strings.Repeat("ab", 32)
	if validateLiveActivityRegistration(request) == "" {
		t.Fatal("a registration with both credentials must be refused")
	}
}

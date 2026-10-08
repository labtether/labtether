package alerting

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/incidents"
	"github.com/labtether/labtether/internal/notifications"
	"github.com/labtether/labtether/internal/persistence"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type liveActivitySecretsStub struct{}

func (liveActivitySecretsStub) EncryptString(plaintext, aad string) (string, error) {
	return "encrypted:" + aad + ":" + base64.RawStdEncoding.EncodeToString([]byte(plaintext)), nil
}

func (liveActivitySecretsStub) DecryptString(ciphertext, aad string) (string, error) {
	prefix := "encrypted:" + aad + ":"
	if !strings.HasPrefix(ciphertext, prefix) {
		return "", errors.New("AAD mismatch")
	}
	decoded, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(ciphertext, prefix))
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

type liveActivityAdapterStub struct {
	mu      sync.Mutex
	pushes  []notifications.LiveActivityPush
	sendErr error
}

type blockingLiveActivityAdapter struct {
	active  atomic.Int64
	maximum atomic.Int64
	sent    atomic.Int64
	started chan struct{}
	release chan struct{}
}

func (a *blockingLiveActivityAdapter) Type() string { return notifications.ChannelTypeAPNs }

func (a *blockingLiveActivityAdapter) Send(context.Context, map[string]any, map[string]any) error {
	return errors.New("normal alert path must not be used")
}

func (a *blockingLiveActivityAdapter) SendLiveActivity(
	ctx context.Context,
	_ map[string]any,
	_ notifications.LiveActivityPush,
) error {
	active := a.active.Add(1)
	defer a.active.Add(-1)
	for {
		maximum := a.maximum.Load()
		if active <= maximum || a.maximum.CompareAndSwap(maximum, active) {
			break
		}
	}
	a.started <- struct{}{}
	select {
	case <-a.release:
		a.sent.Add(1)
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (a *liveActivityAdapterStub) Type() string { return notifications.ChannelTypeAPNs }

func (a *liveActivityAdapterStub) Send(context.Context, map[string]any, map[string]any) error {
	return errors.New("normal alert path must not be used")
}

func (a *liveActivityAdapterStub) SendLiveActivity(_ context.Context, _ map[string]any, push notifications.LiveActivityPush) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.pushes = append(a.pushes, push)
	return a.sendErr
}

func (a *liveActivityAdapterStub) snapshot() []notifications.LiveActivityPush {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]notifications.LiveActivityPush(nil), a.pushes...)
}

func (a *liveActivityAdapterStub) setError(err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sendErr = err
}

func TestIncidentLiveActivityRegistrationEncryptsAndBindsExactOwnership(t *testing.T) {
	deps := newTestAlertingDeps(t)
	store := newLiveActivityStoreStub()
	deps.LiveActivityStore = store
	deps.NotificationSecrets = liveActivitySecretsStub{}
	dispatched := make(chan liveActivityDispatchJobForTest, 1)
	deps.DispatchIncidentLiveActivity = func(incident incidents.Incident, event string) {
		dispatched <- liveActivityDispatchJobForTest{incidentID: incident.ID, event: event}
	}
	incident, err := deps.IncidentStore.CreateIncident(incidents.CreateIncidentRequest{Title: "Database", Severity: "critical"})
	if err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("ab", 32)
	body := `{"device_id":"device-1","push_token":"` + token + `","bundle_id":"com.labtether.mobile","environment":"sandbox","show_full_details":true}`
	request := httptest.NewRequest(
		http.MethodPut,
		"/live-activities/incidents/"+incident.ID+"/activities/activity-1",
		bytes.NewBufferString(body),
	)
	request = request.WithContext(apiv2.ContextWithPrincipal(request.Context(), "user-1", "operator"))
	recorder := httptest.NewRecorder()
	deps.HandleIncidentLiveActivityTokens(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), token) {
		t.Fatal("response exposed ActivityKit push token")
	}
	var response struct {
		RegistrationID string `json:"registration_id"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || response.RegistrationID == "" {
		t.Fatalf("registration response omitted opaque cleanup id: body=%s err=%v", recorder.Body.String(), err)
	}
	select {
	case got := <-dispatched:
		if got.incidentID != incident.ID || got.event != "incident.registered" {
			t.Fatalf("registration queued wrong current state: %#v", got)
		}
	default:
		t.Fatal("registration did not queue current incident reconciliation")
	}
	records := store.snapshot()
	if len(records) != 1 {
		t.Fatalf("records=%d want 1", len(records))
	}
	record := records[0]
	if record.UserID != "user-1" || record.DeviceID != "device-1" || record.ActivityID != "activity-1" || record.IncidentID != incident.ID {
		t.Fatalf("registration binding mismatch: %#v", record)
	}
	if record.TokenCiphertext == token || strings.TrimSpace(record.TokenCiphertext) == "" {
		t.Fatal("token was not encrypted before persistence")
	}
	digest := sha256.Sum256([]byte(token))
	if record.TokenHash != hex.EncodeToString(digest[:]) {
		t.Fatal("token hash mismatch")
	}
	if !record.ShowFullDetails || time.Until(record.ExpiresAt) < 11*time.Hour {
		t.Fatalf("privacy/expiry mismatch: %#v", record)
	}

	wrongUserDelete := httptest.NewRequest(
		http.MethodDelete,
		"/live-activities/incidents/"+incident.ID+"/activities/activity-1?device_id=device-1",
		nil,
	)
	wrongUserDelete = wrongUserDelete.WithContext(apiv2.ContextWithPrincipal(wrongUserDelete.Context(), "user-2", "operator"))
	deleteRecorder := httptest.NewRecorder()
	deps.HandleIncidentLiveActivityTokens(deleteRecorder, wrongUserDelete)
	if deleteRecorder.Code != http.StatusNoContent || len(store.snapshot()) != 1 {
		t.Fatalf("another user removed registration: status=%d records=%d", deleteRecorder.Code, len(store.snapshot()))
	}

	exactDelete := httptest.NewRequest(
		http.MethodDelete,
		"/live-activities/incidents/"+incident.ID+"/activities/activity-1?device_id=device-1&registration_id="+response.RegistrationID,
		nil,
	)
	exactDelete = exactDelete.WithContext(apiv2.ContextWithPrincipal(exactDelete.Context(), "user-1", "operator"))
	exactRecorder := httptest.NewRecorder()
	deps.HandleIncidentLiveActivityTokens(exactRecorder, exactDelete)
	if exactRecorder.Code != http.StatusNoContent || len(store.snapshot()) != 0 {
		t.Fatalf("exact compensating delete failed: status=%d records=%d", exactRecorder.Code, len(store.snapshot()))
	}
}

func TestIncidentLiveActivityRegistrationRejectsMalformedToken(t *testing.T) {
	deps := newTestAlertingDeps(t)
	store := newLiveActivityStoreStub()
	deps.LiveActivityStore = store
	deps.NotificationSecrets = liveActivitySecretsStub{}
	incident, _ := deps.IncidentStore.CreateIncident(incidents.CreateIncidentRequest{Title: "Database", Severity: "critical"})
	request := httptest.NewRequest(
		http.MethodPut,
		"/live-activities/incidents/"+incident.ID+"/activities/activity-1",
		bytes.NewBufferString(`{"device_id":"device-1","push_token":"not-a-token","bundle_id":"com.labtether.mobile","environment":"sandbox"}`),
	)
	request = request.WithContext(apiv2.ContextWithPrincipal(request.Context(), "user-1", "operator"))
	recorder := httptest.NewRecorder()
	deps.HandleIncidentLiveActivityTokens(recorder, request)
	if recorder.Code != http.StatusBadRequest || len(store.snapshot()) != 0 {
		t.Fatalf("status=%d records=%d", recorder.Code, len(store.snapshot()))
	}
}

func TestIncidentLiveActivityRegistrationRejectsAPIKeyPrincipal(t *testing.T) {
	deps := newTestAlertingDeps(t)
	store := newLiveActivityStoreStub()
	deps.LiveActivityStore = store
	deps.NotificationSecrets = liveActivitySecretsStub{}
	incident, _ := deps.IncidentStore.CreateIncident(incidents.CreateIncidentRequest{Title: "Database", Severity: "critical"})
	request := httptest.NewRequest(
		http.MethodPut,
		"/live-activities/incidents/"+incident.ID+"/activities/activity-1",
		bytes.NewBufferString(`{"device_id":"device-1","push_token":"`+strings.Repeat("ab", 32)+`","bundle_id":"com.labtether.mobile","environment":"sandbox"}`),
	)
	ctx := apiv2.ContextWithPrincipal(request.Context(), "user-1", "operator")
	ctx = apiv2.ContextWithAPIKeyID(ctx, "key-restricted")
	request = request.WithContext(ctx)
	recorder := httptest.NewRecorder()
	deps.HandleIncidentLiveActivityTokens(recorder, request)
	if recorder.Code != http.StatusForbidden || len(store.snapshot()) != 0 {
		t.Fatalf("api key principal was allowed to create durable push binding: status=%d records=%d", recorder.Code, len(store.snapshot()))
	}
}

func TestIncidentLiveActivityRegistrationReturnsTooManyRequestsAtQuota(t *testing.T) {
	deps := newTestAlertingDeps(t)
	store := newLiveActivityStoreStub()
	store.upsertErr = persistence.ErrLiveActivityRegistrationLimit
	deps.LiveActivityStore = store
	deps.NotificationSecrets = liveActivitySecretsStub{}
	incident, err := deps.IncidentStore.CreateIncident(incidents.CreateIncidentRequest{
		Title: "Database", Severity: incidents.SeverityCritical,
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(
		http.MethodPut,
		"/live-activities/incidents/"+incident.ID+"/activities/activity-1",
		bytes.NewBufferString(`{"device_id":"device-1","push_token":"`+strings.Repeat("ab", 32)+`","bundle_id":"com.labtether.mobile","environment":"sandbox"}`),
	)
	request = request.WithContext(apiv2.ContextWithPrincipal(request.Context(), "user-1", "operator"))
	recorder := httptest.NewRecorder()
	deps.HandleIncidentLiveActivityTokens(recorder, request)
	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("status=%d want %d body=%s", recorder.Code, http.StatusTooManyRequests, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), strings.Repeat("ab", 32)) {
		t.Fatal("quota response exposed ActivityKit token")
	}
}

func TestIncidentLiveActivityDeregistrationDoesNotRequireIncidentOrEncryptionStores(t *testing.T) {
	store := newLiveActivityStoreStub()
	store.records["lat-orphan"] = persistence.LiveActivityPushToken{
		ID: "lat-orphan", UserID: "user-1", DeviceID: "device-1",
		ActivityID: "activity-1", IncidentID: "deleted-incident",
	}
	deps := &Deps{LiveActivityStore: store}
	request := httptest.NewRequest(
		http.MethodDelete,
		"/live-activities/incidents/deleted-incident/activities/activity-1?device_id=device-1",
		nil,
	)
	request = request.WithContext(apiv2.ContextWithPrincipal(request.Context(), "user-1", "operator"))
	recorder := httptest.NewRecorder()
	deps.HandleIncidentLiveActivityTokens(recorder, request)
	if recorder.Code != http.StatusNoContent || len(store.snapshot()) != 0 {
		t.Fatalf("status=%d records=%d", recorder.Code, len(store.snapshot()))
	}
}

func TestIncidentLiveActivityContentChangeIgnoresPostmortemOnlyFields(t *testing.T) {
	base := incidents.Incident{ID: "incident", Title: "Outage", Summary: "Investigating", Assignee: "Alice", Status: "open", Severity: "high", OpenedAt: time.Now().UTC()}
	updated := base
	updated.Title = "Database outage"
	if !incidentLiveActivityContentChanged(base, updated) {
		t.Fatal("title change should refresh Live Activity")
	}
	updated = base
	updated.RootCause = "Cable failure"
	if incidentLiveActivityContentChanged(base, updated) {
		t.Fatal("postmortem-only field should not refresh Live Activity content")
	}
}

func TestIncidentLiveActivityDeliveryUsesPrivacyRoleAndDedicatedSender(t *testing.T) {
	deps := newTestAlertingDeps(t)
	store := newLiveActivityStoreStub()
	secrets := liveActivitySecretsStub{}
	deps.LiveActivityStore = store
	deps.NotificationSecrets = secrets
	deps.LiveActivityUserCanMutate = func(userID string) bool { return userID == "operator" }
	channelStore := newNotificationSecurityStore()
	channelStore.seed(notifications.Channel{
		ID: "apns", Type: notifications.ChannelTypeAPNs, Enabled: true,
		Config: map[string]any{"bundle_id": "com.labtether.mobile", "production": false, "auth_key_path": "test.p8", "key_id": "KEYID12345", "team_id": "TEAMID1234"},
	})
	deps.NotificationStore = channelStore
	adapter := &liveActivityAdapterStub{}
	deps.NotificationAdapters = map[string]notifications.Adapter{notifications.ChannelTypeAPNs: adapter}
	now := time.Now().UTC()
	incident, err := deps.IncidentStore.CreateIncident(incidents.CreateIncidentRequest{
		Title: "Secret database outage", Summary: "Secret details", Assignee: "Alice",
		Severity: incidents.SeverityCritical,
	})
	if err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("cd", 32)
	digest := sha256.Sum256([]byte(token))
	record := persistence.LiveActivityPushToken{
		ID: "lat-1", UserID: "operator", DeviceID: "device-1", ActivityID: "activity-1", IncidentID: incident.ID,
		TokenHash: hex.EncodeToString(digest[:]), BundleID: "com.labtether.mobile", Environment: "sandbox",
		ExpiresAt: now.Add(time.Hour), ShowFullDetails: false,
	}
	record.TokenCiphertext, _ = secrets.EncryptString(token, liveActivityTokenAAD(record.ID))
	_ = store.UpsertLiveActivityPushToken(context.Background(), record)
	deps.DeliverIncidentLiveActivity(incident, "incident.status_changed")
	pushes := adapter.snapshot()
	if len(pushes) != 1 {
		t.Fatalf("pushes=%d want 1", len(pushes))
	}
	push := pushes[0]
	if push.Event != "update" || push.State.Title != "Incident in progress" || push.State.Summary != "" || push.State.Assignee != "" {
		t.Fatalf("privacy-safe push mismatch: %#v", push)
	}
	if !push.State.CanMutate || push.DeviceToken != token {
		t.Fatalf("role/token routing mismatch: %#v", push)
	}
	if retained := store.snapshot(); len(retained) != 1 || retained[0].RetryCount != 0 || retained[0].NextRetryAt != nil {
		t.Fatalf("successful delivery retained retry state: %#v", retained)
	}
}

func TestIncidentTransitionSchedulesLiveActivityWithoutNotificationStore(t *testing.T) {
	called := make(chan liveActivityDispatchJobForTest, 1)
	deps := &Deps{
		DispatchIncidentLiveActivity: func(incident incidents.Incident, event string) {
			called <- liveActivityDispatchJobForTest{incidentID: incident.ID, event: event}
		},
	}
	deps.dispatchIncidentNotificationAsync(
		incidents.Incident{ID: "incident-live-only", Title: "Outage", Status: incidents.StatusOpen, Severity: incidents.SeverityCritical},
		"incident.status_changed",
	)
	select {
	case got := <-called:
		if got.incidentID != "incident-live-only" || got.event != "incident.status_changed" {
			t.Fatalf("unexpected callback: %#v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("live activity callback was not scheduled")
	}
}

type liveActivityDispatchJobForTest struct {
	incidentID string
	event      string
}

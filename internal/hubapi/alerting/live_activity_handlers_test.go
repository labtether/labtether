package alerting

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/incidents"
	"github.com/labtether/labtether/internal/notifications"
	"github.com/labtether/labtether/internal/persistence"
)

type liveActivityStoreStub struct {
	mu        sync.Mutex
	records   map[string]persistence.LiveActivityPushToken
	reconcile map[string]bool
	upsertErr error
}

func newLiveActivityStoreStub() *liveActivityStoreStub {
	return &liveActivityStoreStub{
		records:   make(map[string]persistence.LiveActivityPushToken),
		reconcile: make(map[string]bool),
	}
}

func (s *liveActivityStoreStub) UpsertLiveActivityPushToken(_ context.Context, token persistence.LiveActivityPushToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.upsertErr != nil {
		return s.upsertErr
	}
	for id, existing := range s.records {
		if (existing.UserID == token.UserID && existing.DeviceID == token.DeviceID && existing.ActivityID == token.ActivityID) ||
			(existing.TokenHash == token.TokenHash && existing.BundleID == token.BundleID && existing.Environment == token.Environment) {
			delete(s.records, id)
		}
	}
	s.records[token.ID] = token
	return nil
}

func (s *liveActivityStoreStub) DeleteLiveActivityPushTokenByOwnerAndID(
	_ context.Context,
	userID, deviceID, activityID, incidentID, registrationID string,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	token, found := s.records[registrationID]
	if found && token.UserID == userID && token.DeviceID == deviceID &&
		token.ActivityID == activityID && token.IncidentID == incidentID {
		delete(s.records, registrationID)
	}
	return nil
}

func (s *liveActivityStoreStub) DeleteLiveActivityPushToken(_ context.Context, userID, deviceID, activityID, incidentID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, token := range s.records {
		if token.UserID == userID && token.DeviceID == deviceID && token.ActivityID == activityID && token.IncidentID == incidentID {
			delete(s.records, id)
		}
	}
	return nil
}

func (s *liveActivityStoreStub) DeleteLiveActivityPushTokenByID(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.records, id)
	return nil
}

func (s *liveActivityStoreStub) DeleteLiveActivityPushTokenByGeneration(_ context.Context, id string, generation int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if token, found := s.records[id]; found && token.DeliveryGeneration == generation {
		delete(s.records, id)
	}
	return nil
}

func (s *liveActivityStoreStub) ListLiveActivityPushTokensForIncident(_ context.Context, incidentID string, now time.Time) ([]persistence.LiveActivityPushToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]persistence.LiveActivityPushToken, 0)
	for _, token := range s.records {
		if token.IncidentID == incidentID && token.ExpiresAt.After(now) {
			result = append(result, token)
		}
	}
	return result, nil
}

func (s *liveActivityStoreStub) ListDueLiveActivityPushTokens(_ context.Context, now time.Time, limit int) ([]persistence.LiveActivityPushToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]persistence.LiveActivityPushToken, 0)
	for _, token := range s.records {
		if token.ExpiresAt.After(now) && token.NextRetryAt != nil && !token.NextRetryAt.After(now) {
			result = append(result, token)
			if len(result) == limit {
				break
			}
		}
	}
	return result, nil
}

func (s *liveActivityStoreStub) ListLiveActivityPushTokensForReconciliation(_ context.Context, now time.Time, limit int) ([]persistence.LiveActivityPushToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]persistence.LiveActivityPushToken, 0)
	for id, token := range s.records {
		if s.reconcile[id] && token.ExpiresAt.After(now) && (token.NextRetryAt == nil || !token.NextRetryAt.After(now)) {
			result = append(result, token)
			delete(s.reconcile, id)
			if len(result) == limit {
				break
			}
		}
	}
	return result, nil
}

func (s *liveActivityStoreStub) ClaimLiveActivityPushDelivery(
	_ context.Context,
	id string,
	expectedGeneration int64,
	pendingState string,
	_ time.Time,
	leaseUntil time.Time,
	retryCount int,
) (int64, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	token, found := s.records[id]
	if !found || !token.ExpiresAt.After(time.Now().UTC()) ||
		(expectedGeneration >= 0 && token.DeliveryGeneration != expectedGeneration) {
		return 0, false, nil
	}
	token.DeliveryGeneration++
	token.RetryCount = retryCount
	token.NextRetryAt = &leaseUntil
	token.PendingStateCiphertext = pendingState
	token.UpdatedAt = time.Now().UTC()
	s.records[id] = token
	return token.DeliveryGeneration, true, nil
}

func (s *liveActivityStoreStub) MarkLiveActivityPushRetry(
	_ context.Context,
	id string,
	generation int64,
	count int,
	next time.Time,
	pendingState string,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	token, found := s.records[id]
	if !found || token.DeliveryGeneration != generation {
		return nil
	}
	token.RetryCount = count
	token.NextRetryAt = &next
	token.PendingStateCiphertext = pendingState
	token.UpdatedAt = time.Now().UTC()
	s.records[id] = token
	return nil
}

func (s *liveActivityStoreStub) ClearLiveActivityPushRetry(
	_ context.Context,
	id string,
	generation int64,
	deliveredIncidentUpdatedAt time.Time,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	token, found := s.records[id]
	if !found || token.DeliveryGeneration != generation {
		return nil
	}
	token.RetryCount = 0
	token.NextRetryAt = nil
	token.PendingStateCiphertext = ""
	token.LastDeliveredIncidentUpdatedAt = &deliveredIncidentUpdatedAt
	token.UpdatedAt = time.Now().UTC()
	s.records[id] = token
	return nil
}

func (s *liveActivityStoreStub) DeleteExpiredLiveActivityPushTokens(_ context.Context, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, token := range s.records {
		if !token.ExpiresAt.After(now) {
			delete(s.records, id)
		}
	}
	return nil
}

func (s *liveActivityStoreStub) snapshot() []persistence.LiveActivityPushToken {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]persistence.LiveActivityPushToken, 0, len(s.records))
	for _, token := range s.records {
		result = append(result, token)
	}
	return result
}

func (s *liveActivityStoreStub) makeRetryDue(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	token := s.records[id]
	due := time.Now().UTC().Add(-time.Second)
	token.NextRetryAt = &due
	s.records[id] = token
}

func (s *liveActivityStoreStub) markForReconciliation(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reconcile[id] = true
}

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

func TestIncidentLiveActivityTransientFailureRetriesAndResolvedCanReopenBeforeClose(t *testing.T) {
	deps := newTestAlertingDeps(t)
	store := newLiveActivityStoreStub()
	secrets := liveActivitySecretsStub{}
	deps.LiveActivityStore = store
	deps.NotificationSecrets = secrets
	channelStore := newNotificationSecurityStore()
	channelStore.seed(notifications.Channel{
		ID: "apns", Type: notifications.ChannelTypeAPNs, Enabled: true,
		Config: map[string]any{"bundle_id": "com.labtether.mobile", "production": true, "auth_key_path": "test.p8", "key_id": "KEYID12345", "team_id": "TEAMID1234"},
	})
	deps.NotificationStore = channelStore
	adapter := &liveActivityAdapterStub{sendErr: errors.New("temporary APNs outage")}
	deps.NotificationAdapters = map[string]notifications.Adapter{notifications.ChannelTypeAPNs: adapter}
	now := time.Now().UTC()
	token := strings.Repeat("ef", 32)
	digest := sha256.Sum256([]byte(token))
	incident, err := deps.IncidentStore.CreateIncident(incidents.CreateIncidentRequest{
		Title: "Incident", Severity: incidents.SeverityHigh,
	})
	if err != nil {
		t.Fatal(err)
	}
	record := persistence.LiveActivityPushToken{
		ID: "lat-2", UserID: "viewer", DeviceID: "device-2", ActivityID: "activity-2", IncidentID: incident.ID,
		TokenHash: hex.EncodeToString(digest[:]), BundleID: "com.labtether.mobile", Environment: "production",
		ExpiresAt: now.Add(time.Hour),
	}
	record.TokenCiphertext, _ = secrets.EncryptString(token, liveActivityTokenAAD(record.ID))
	_ = store.UpsertLiveActivityPushToken(context.Background(), record)
	deps.DeliverIncidentLiveActivity(incident, "incident.severity_changed")
	retrying := store.snapshot()
	if len(retrying) != 1 || retrying[0].RetryCount != 1 || retrying[0].NextRetryAt == nil || retrying[0].PendingStateCiphertext == "" {
		t.Fatalf("transient failure did not schedule retry: %#v", retrying)
	}
	if strings.Contains(retrying[0].PendingStateCiphertext, incident.Title) {
		t.Fatal("pending incident retry state was stored in plaintext")
	}

	adapter.setError(nil)
	resolved := incidents.StatusResolved
	incident, err = deps.IncidentStore.UpdateIncident(incident.ID, incidents.UpdateIncidentRequest{Status: &resolved})
	if err != nil {
		t.Fatal(err)
	}
	deps.DeliverIncidentLiveActivity(incident, "incident.resolved")
	if len(store.snapshot()) != 1 {
		t.Fatal("resolved incident must retain its token so it can reopen")
	}
	pushes := adapter.snapshot()
	if pushes[len(pushes)-1].Event != "update" || pushes[len(pushes)-1].State.Status != incidents.StatusResolved {
		t.Fatalf("resolved incident should remain updateable: %#v", pushes[len(pushes)-1])
	}

	investigating := incidents.StatusInvestigating
	incident, err = deps.IncidentStore.UpdateIncident(incident.ID, incidents.UpdateIncidentRequest{Status: &investigating})
	if err != nil {
		t.Fatal(err)
	}
	deps.DeliverIncidentLiveActivity(incident, "incident.status_changed")
	pushes = adapter.snapshot()
	if pushes[len(pushes)-1].Event != "update" || pushes[len(pushes)-1].State.Status != incidents.StatusInvestigating {
		t.Fatalf("reopened incident did not reuse the current Activity: %#v", pushes[len(pushes)-1])
	}

	closed := incidents.StatusClosed
	incident, err = deps.IncidentStore.UpdateIncident(incident.ID, incidents.UpdateIncidentRequest{Status: &closed})
	if err != nil {
		t.Fatal(err)
	}
	deps.DeliverIncidentLiveActivity(incident, "incident.status_changed")
	if len(store.snapshot()) != 0 {
		t.Fatal("closed incident did not remove ActivityKit token")
	}
	pushes = adapter.snapshot()
	if pushes[len(pushes)-1].Event != "end" || pushes[len(pushes)-1].DismissAt == nil {
		t.Fatalf("closed incident did not send ActivityKit end: %#v", pushes[len(pushes)-1])
	}
}

func TestDeletedIncidentTerminalStateRetriesFromEncryptedSnapshot(t *testing.T) {
	deps := newTestAlertingDeps(t)
	store := newLiveActivityStoreStub()
	secrets := liveActivitySecretsStub{}
	deps.LiveActivityStore = store
	deps.NotificationSecrets = secrets
	channelStore := newNotificationSecurityStore()
	channelStore.seed(notifications.Channel{
		ID: "apns", Type: notifications.ChannelTypeAPNs, Enabled: true,
		Config: map[string]any{"bundle_id": "com.labtether.mobile", "production": false, "auth_key_path": "test.p8", "key_id": "KEYID12345", "team_id": "TEAMID1234"},
	})
	deps.NotificationStore = channelStore
	adapter := &liveActivityAdapterStub{sendErr: errors.New("temporary APNs outage")}
	deps.NotificationAdapters = map[string]notifications.Adapter{notifications.ChannelTypeAPNs: adapter}
	now := time.Now().UTC()
	token := strings.Repeat("12", 32)
	digest := sha256.Sum256([]byte(token))
	record := persistence.LiveActivityPushToken{
		ID: "lat-terminal", UserID: "viewer", DeviceID: "device-terminal", ActivityID: "activity-terminal", IncidentID: "deleted-incident",
		TokenHash: hex.EncodeToString(digest[:]), BundleID: "com.labtether.mobile", Environment: "sandbox",
		ExpiresAt: now.Add(time.Hour),
	}
	record.TokenCiphertext, _ = secrets.EncryptString(token, liveActivityTokenAAD(record.ID))
	_ = store.UpsertLiveActivityPushToken(context.Background(), record)
	finalIncident := incidents.Incident{
		ID: "deleted-incident", Title: "Final outage state", Status: incidents.StatusClosed,
		Severity: incidents.SeverityCritical, OpenedAt: now.Add(-time.Hour), UpdatedAt: now,
	}
	deps.DeliverIncidentLiveActivity(finalIncident, "incident.resolved")
	failed := store.snapshot()
	if len(failed) != 1 || failed[0].PendingStateCiphertext == "" {
		t.Fatalf("terminal state was not retained for retry: %#v", failed)
	}
	store.makeRetryDue(record.ID)
	adapter.setError(nil)
	// The memory incident store deliberately has no deleted-incident row; retry
	// must use the encrypted final snapshot rather than dropping the end event.
	deps.retryDueLiveActivityPushes(context.Background())
	if len(store.snapshot()) != 0 {
		t.Fatal("successful terminal retry did not remove registration")
	}
	pushes := adapter.snapshot()
	if len(pushes) != 2 || pushes[1].Event != "end" || pushes[1].State.Status != incidents.StatusClosed {
		t.Fatalf("terminal retry did not preserve final state: %#v", pushes)
	}
}

func TestMissingIncidentRetryConvertsStaleOpenStateToPrivacySafeEnd(t *testing.T) {
	deps := newTestAlertingDeps(t)
	store := newLiveActivityStoreStub()
	secrets := liveActivitySecretsStub{}
	deps.LiveActivityStore = store
	deps.NotificationSecrets = secrets
	channelStore := newNotificationSecurityStore()
	channelStore.seed(notifications.Channel{
		ID: "apns", Type: notifications.ChannelTypeAPNs, Enabled: true,
		Config: map[string]any{"bundle_id": "com.labtether.mobile", "production": false, "auth_key_path": "test.p8", "key_id": "KEYID12345", "team_id": "TEAMID1234"},
	})
	deps.NotificationStore = channelStore
	adapter := &liveActivityAdapterStub{}
	deps.NotificationAdapters = map[string]notifications.Adapter{notifications.ChannelTypeAPNs: adapter}
	now := time.Now().UTC()
	token := strings.Repeat("34", 32)
	digest := sha256.Sum256([]byte(token))
	record := persistence.LiveActivityPushToken{
		ID: "lat-stale-open", UserID: "viewer", DeviceID: "device-stale", ActivityID: "activity-stale", IncidentID: "deleted-open-incident",
		TokenHash: hex.EncodeToString(digest[:]), BundleID: "com.labtether.mobile", Environment: "sandbox",
		ExpiresAt: now.Add(time.Hour), RetryCount: 1,
	}
	record.TokenCiphertext, _ = secrets.EncryptString(token, liveActivityTokenAAD(record.ID))
	record.PendingStateCiphertext, _ = deps.encryptPendingLiveActivityIncident(record.ID, incidents.Incident{
		ID: record.IncidentID, Title: "Sensitive stale title", Summary: "Sensitive stale summary",
		Status: incidents.StatusOpen, Severity: incidents.SeverityHigh, Assignee: "Alice",
		OpenedAt: now.Add(-time.Hour), CreatedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Minute),
	})
	due := now.Add(-time.Second)
	record.NextRetryAt = &due
	_ = store.UpsertLiveActivityPushToken(context.Background(), record)

	deps.retryDueLiveActivityPushes(context.Background())
	if len(store.snapshot()) != 0 {
		t.Fatal("privacy-safe end did not remove deleted incident registration")
	}
	pushes := adapter.snapshot()
	if len(pushes) != 1 || pushes[0].Event != "end" || pushes[0].State.Status != incidents.StatusClosed ||
		pushes[0].State.Title != "Incident in progress" || pushes[0].State.Summary != "" || pushes[0].State.Assignee != "" {
		t.Fatalf("missing incident did not fail closed: %#v", pushes)
	}

	// Also prove the durable reconciliation scan closes an orphan even when a
	// crash dropped the hard-delete callback before any pending retry was stored.
	orphan := record
	orphan.ID = "lat-lost-delete-callback"
	orphan.ActivityID = "activity-lost-delete"
	orphan.RetryCount = 0
	orphan.NextRetryAt = nil
	orphan.PendingStateCiphertext = ""
	orphan.TokenCiphertext, _ = secrets.EncryptString(token, liveActivityTokenAAD(orphan.ID))
	_ = store.UpsertLiveActivityPushToken(context.Background(), orphan)
	store.markForReconciliation(orphan.ID)
	deps.reconcileCommittedLiveActivityState(context.Background(), now)
	if len(store.snapshot()) != 0 {
		t.Fatal("reconciliation did not remove orphaned hard-delete registration")
	}
	pushes = adapter.snapshot()
	if len(pushes) != 2 || pushes[1].Event != "end" || pushes[1].State.Status != incidents.StatusClosed {
		t.Fatalf("orphan reconciliation did not emit terminal state: %#v", pushes)
	}
}

func TestLiveActivityGenerationFenceRejectsStaleRetryCompletion(t *testing.T) {
	store := newLiveActivityStoreStub()
	now := time.Now().UTC()
	record := persistence.LiveActivityPushToken{ID: "lat-generation", ExpiresAt: now.Add(time.Hour)}
	_ = store.UpsertLiveActivityPushToken(context.Background(), record)

	oldGeneration, claimed, err := store.ClaimLiveActivityPushDelivery(
		context.Background(), record.ID, 0, "encrypted-old", now, now.Add(time.Minute), 1,
	)
	if err != nil || !claimed {
		t.Fatalf("old retry claim failed: claimed=%v err=%v", claimed, err)
	}
	newGeneration, claimed, err := store.ClaimLiveActivityPushDelivery(
		context.Background(), record.ID, -1, "encrypted-terminal", now, now.Add(2*time.Minute), 0,
	)
	if err != nil || !claimed || newGeneration <= oldGeneration {
		t.Fatalf("new terminal claim failed: old=%d new=%d claimed=%v err=%v", oldGeneration, newGeneration, claimed, err)
	}
	_ = store.ClearLiveActivityPushRetry(context.Background(), record.ID, oldGeneration, now)
	_ = store.MarkLiveActivityPushRetry(
		context.Background(), record.ID, oldGeneration, 5, now.Add(5*time.Minute), "encrypted-old-overwrite",
	)
	remaining := store.snapshot()
	if len(remaining) != 1 || remaining[0].DeliveryGeneration != newGeneration ||
		remaining[0].PendingStateCiphertext != "encrypted-terminal" || remaining[0].NextRetryAt == nil {
		t.Fatalf("stale retry completion overwrote terminal desired state: %#v", remaining)
	}
}

func TestTransientLiveActivityFailureRetainsValidTokenAtRetryCap(t *testing.T) {
	deps := newTestAlertingDeps(t)
	store := newLiveActivityStoreStub()
	secrets := liveActivitySecretsStub{}
	deps.LiveActivityStore = store
	deps.NotificationSecrets = secrets
	channelStore := newNotificationSecurityStore()
	channelStore.seed(notifications.Channel{
		ID: "apns", Type: notifications.ChannelTypeAPNs, Enabled: true,
		Config: map[string]any{"bundle_id": "com.labtether.mobile", "production": false, "auth_key_path": "test.p8", "key_id": "KEYID12345", "team_id": "TEAMID1234"},
	})
	deps.NotificationStore = channelStore
	deps.NotificationAdapters = map[string]notifications.Adapter{
		notifications.ChannelTypeAPNs: &liveActivityAdapterStub{sendErr: errors.New("provider unavailable")},
	}
	incident, err := deps.IncidentStore.CreateIncident(incidents.CreateIncidentRequest{Title: "Outage", Severity: incidents.SeverityHigh})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	token := strings.Repeat("56", 32)
	digest := sha256.Sum256([]byte(token))
	record := persistence.LiveActivityPushToken{
		ID: "lat-retry-cap", UserID: "viewer", DeviceID: "device", ActivityID: "activity", IncidentID: incident.ID,
		TokenHash: hex.EncodeToString(digest[:]), TokenCiphertext: "", BundleID: "com.labtether.mobile", Environment: "sandbox",
		ExpiresAt: now.Add(time.Hour), RetryCount: liveActivityMaxRetryCount,
	}
	record.TokenCiphertext, _ = secrets.EncryptString(token, liveActivityTokenAAD(record.ID))
	_ = store.UpsertLiveActivityPushToken(context.Background(), record)

	deps.DeliverIncidentLiveActivity(incident, "retry")
	remaining := store.snapshot()
	if len(remaining) != 1 || remaining[0].RetryCount != liveActivityMaxRetryCount || remaining[0].NextRetryAt == nil {
		t.Fatalf("transient failure discarded or stopped retrying valid token: %#v", remaining)
	}
}

func TestLiveActivityFanoutUsesFixedWorkerBound(t *testing.T) {
	deps := newTestAlertingDeps(t)
	store := newLiveActivityStoreStub()
	secrets := liveActivitySecretsStub{}
	deps.LiveActivityStore = store
	deps.NotificationSecrets = secrets
	channelStore := newNotificationSecurityStore()
	channelStore.seed(notifications.Channel{
		ID: "apns", Type: notifications.ChannelTypeAPNs, Enabled: true,
		Config: map[string]any{"bundle_id": "com.labtether.mobile", "production": false, "auth_key_path": "test.p8", "key_id": "KEYID12345", "team_id": "TEAMID1234"},
	})
	deps.NotificationStore = channelStore
	const registrationCount = 40
	adapter := &blockingLiveActivityAdapter{
		started: make(chan struct{}, registrationCount),
		release: make(chan struct{}),
	}
	deps.NotificationAdapters = map[string]notifications.Adapter{notifications.ChannelTypeAPNs: adapter}
	incident, err := deps.IncidentStore.CreateIncident(incidents.CreateIncidentRequest{Title: "Outage", Severity: incidents.SeverityHigh})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for index := 0; index < registrationCount; index++ {
		token := fmt.Sprintf("%064x", index+1)
		digest := sha256.Sum256([]byte(token))
		record := persistence.LiveActivityPushToken{
			ID: fmt.Sprintf("lat-worker-%d", index), UserID: fmt.Sprintf("user-%d", index), DeviceID: "device", ActivityID: "activity", IncidentID: incident.ID,
			TokenHash: hex.EncodeToString(digest[:]), BundleID: "com.labtether.mobile", Environment: "sandbox", ExpiresAt: now.Add(time.Hour),
		}
		record.TokenCiphertext, _ = secrets.EncryptString(token, liveActivityTokenAAD(record.ID))
		_ = store.UpsertLiveActivityPushToken(context.Background(), record)
	}
	done := make(chan struct{})
	go func() {
		deps.DeliverIncidentLiveActivity(incident, "incident.updated")
		close(done)
	}()
	for index := 0; index < 8; index++ {
		select {
		case <-adapter.started:
		case <-time.After(time.Second):
			t.Fatalf("worker %d did not start", index+1)
		}
	}
	runtime.Gosched()
	select {
	case <-adapter.started:
		t.Fatal("fanout started more than eight blocked APNs deliveries")
	case <-time.After(25 * time.Millisecond):
	}
	close(adapter.release)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("bounded fanout did not complete")
	}
	if maximum := adapter.maximum.Load(); maximum > 8 || adapter.sent.Load() != registrationCount {
		t.Fatalf("fanout bound/sent mismatch: maximum=%d sent=%d", maximum, adapter.sent.Load())
	}
}

func TestLiveActivityRetryClaimIsSingleWinnerAndGenerationFenced(t *testing.T) {
	store := newLiveActivityStoreStub()
	now := time.Now().UTC()
	store.records["lat-claim"] = persistence.LiveActivityPushToken{
		ID: "lat-claim", IncidentID: "incident-claim", ExpiresAt: now.Add(time.Hour),
	}

	start := make(chan struct{})
	winners := make(chan int64, 2)
	var attempts sync.WaitGroup
	attempts.Add(2)
	for attempt := 0; attempt < 2; attempt++ {
		attempt := attempt
		go func() {
			defer attempts.Done()
			<-start
			generation, claimed, err := store.ClaimLiveActivityPushDelivery(
				context.Background(), "lat-claim", 0,
				fmt.Sprintf("pending-%d", attempt), now, now.Add(liveActivityDeliveryLease), 1,
			)
			if err != nil {
				t.Errorf("claim failed: %v", err)
				return
			}
			if claimed {
				winners <- generation
			}
		}()
	}
	close(start)
	attempts.Wait()
	close(winners)
	var firstGeneration int64
	winnerCount := 0
	for generation := range winners {
		winnerCount++
		firstGeneration = generation
	}
	if winnerCount != 1 || firstGeneration != 1 {
		t.Fatalf("claim winners=%d generation=%d, want one winner at generation 1", winnerCount, firstGeneration)
	}

	newLease := now.Add(2 * liveActivityDeliveryLease)
	secondGeneration, claimed, err := store.ClaimLiveActivityPushDelivery(
		context.Background(), "lat-claim", firstGeneration, "newest-state", now, newLease, 7,
	)
	if err != nil || !claimed || secondGeneration != firstGeneration+1 {
		t.Fatalf("newer claim=(generation=%d claimed=%t err=%v)", secondGeneration, claimed, err)
	}
	staleNext := now.Add(24 * time.Hour)
	if err := store.MarkLiveActivityPushRetry(context.Background(), "lat-claim", firstGeneration, 99, staleNext, "stale-state"); err != nil {
		t.Fatal(err)
	}
	if err := store.ClearLiveActivityPushRetry(context.Background(), "lat-claim", firstGeneration, now); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteLiveActivityPushTokenByGeneration(context.Background(), "lat-claim", firstGeneration); err != nil {
		t.Fatal(err)
	}
	records := store.snapshot()
	if len(records) != 1 {
		t.Fatalf("stale generation removed current registration: %#v", records)
	}
	got := records[0]
	if got.DeliveryGeneration != secondGeneration || got.RetryCount != 7 ||
		got.PendingStateCiphertext != "newest-state" || got.NextRetryAt == nil || !got.NextRetryAt.Equal(newLease) {
		t.Fatalf("stale completion overwrote current generation: %#v", got)
	}
}

func TestLiveActivityDeliveryUsesFixedWorkerPool(t *testing.T) {
	const registrationCount = 256

	deps := newTestAlertingDeps(t)
	store := newLiveActivityStoreStub()
	secrets := liveActivitySecretsStub{}
	deps.LiveActivityStore = store
	deps.NotificationSecrets = secrets
	channelStore := newNotificationSecurityStore()
	channelStore.seed(notifications.Channel{
		ID: "apns", Type: notifications.ChannelTypeAPNs, Enabled: true,
		Config: map[string]any{"bundle_id": "com.labtether.mobile", "production": false, "auth_key_path": "test.p8", "key_id": "KEYID12345", "team_id": "TEAMID1234"},
	})
	deps.NotificationStore = channelStore
	adapter := &blockingLiveActivityAdapter{
		started: make(chan struct{}, registrationCount),
		release: make(chan struct{}),
	}
	deps.NotificationAdapters = map[string]notifications.Adapter{notifications.ChannelTypeAPNs: adapter}

	now := time.Now().UTC()
	registrations := make([]persistence.LiveActivityPushToken, 0, registrationCount)
	for i := 0; i < registrationCount; i++ {
		token := fmt.Sprintf("%064x", i+1)
		digest := sha256.Sum256([]byte(token))
		record := persistence.LiveActivityPushToken{
			ID: fmt.Sprintf("lat-worker-%d", i), UserID: "operator", DeviceID: fmt.Sprintf("device-%d", i),
			ActivityID: fmt.Sprintf("activity-%d", i), IncidentID: "incident-workers",
			TokenHash: hex.EncodeToString(digest[:]), BundleID: "com.labtether.mobile", Environment: "sandbox",
			ExpiresAt: now.Add(time.Hour),
		}
		record.TokenCiphertext, _ = secrets.EncryptString(token, liveActivityTokenAAD(record.ID))
		if err := store.UpsertLiveActivityPushToken(context.Background(), record); err != nil {
			t.Fatal(err)
		}
		registrations = append(registrations, record)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(adapter.release) }) }
	defer release()
	baselineGoroutines := runtime.NumGoroutine()
	done := make(chan struct{})
	go func() {
		defer close(done)
		deps.deliverLiveActivityRegistrations(
			ctx,
			incidents.Incident{ID: "incident-workers", Title: "Outage", Status: incidents.StatusOpen, Severity: incidents.SeverityHigh, OpenedAt: now, UpdatedAt: now},
			"incident.status_changed",
			registrations,
			now,
		)
	}()
	for i := 0; i < 8; i++ {
		select {
		case <-adapter.started:
		case <-time.After(time.Second):
			release()
			<-done
			t.Fatalf("only %d delivery workers reached the blocking sender", i)
		}
	}
	if delta := runtime.NumGoroutine() - baselineGoroutines; delta > 32 {
		release()
		<-done
		t.Fatalf("delivery created %d goroutines for %d registrations", delta, registrationCount)
	}
	release()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("bounded delivery workers did not drain")
	}
	if maximum := adapter.maximum.Load(); maximum > 8 {
		t.Fatalf("maximum concurrent deliveries=%d want <=8", maximum)
	}
	if sent := adapter.sent.Load(); sent != registrationCount {
		t.Fatalf("sent=%d want %d", sent, registrationCount)
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

package alerting

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/labtether/labtether/internal/incidents"
	"github.com/labtether/labtether/internal/notifications"
	"github.com/labtether/labtether/internal/persistence"
	"runtime"
	"sync"
	"testing"
	"time"
)

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

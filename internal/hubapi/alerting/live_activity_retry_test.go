package alerting

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/labtether/labtether/internal/incidents"
	"github.com/labtether/labtether/internal/notifications"
	"github.com/labtether/labtether/internal/persistence"
	"strings"
	"sync"
	"testing"
	"time"
)

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

package persistence

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/notifications"
)

func pushDigestTestDevice(userID, suffix string) PushDevice {
	return PushDevice{
		UserID:                 userID,
		DeviceID:               "device-" + suffix,
		Platform:               "ios",
		PushToken:              "token-" + suffix,
		BundleID:               "com.labtether.qa",
		Environment:            "sandbox",
		TimeZone:               "UTC",
		NotifyCriticalAlerts:   true,
		NotifyNodeOffline:      true,
		NotifyServiceDown:      true,
		PushCategory:           "all_alerts",
		MinimumSeverity:        "warning",
		QuietHoursStartMinutes: 22 * 60,
		QuietHoursEndMinutes:   7 * 60,
		DigestWindowSeconds:    30,
	}
}

func TestPostgresPushDeviceQuotaAdmissionAndDeleteRecoveryAreAtomic(t *testing.T) {
	store := newTestPostgresStore(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UTC().UnixNano())
	userID := "push-quota-user-" + suffix
	t.Cleanup(func() {
		_, _ = store.pool.Exec(context.Background(), `DELETE FROM push_devices WHERE user_id = $1`, userID)
	})

	for index := 0; index < MaxPushDevicesPerUser-1; index++ {
		device := pushDigestTestDevice(userID, fmt.Sprintf("%s-%d", suffix, index))
		if err := store.UpsertPushDevice(ctx, device); err != nil {
			t.Fatalf("seed push registration %d: %v", index, err)
		}
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for index := MaxPushDevicesPerUser - 1; index <= MaxPushDevicesPerUser; index++ {
		workers.Add(1)
		go func(index int) {
			defer workers.Done()
			<-start
			results <- store.UpsertPushDevice(ctx, pushDigestTestDevice(userID, fmt.Sprintf("%s-%d", suffix, index)))
		}(index)
	}
	close(start)
	workers.Wait()
	close(results)

	var admitted, rejected int
	for err := range results {
		switch {
		case err == nil:
			admitted++
		case errors.Is(err, ErrPushDeviceRegistrationLimit):
			rejected++
		default:
			t.Fatalf("unexpected concurrent admission result: %v", err)
		}
	}
	if admitted != 1 || rejected != 1 {
		t.Fatalf("concurrent admissions admitted=%d rejected=%d, want 1/1", admitted, rejected)
	}

	var count int
	if err := store.pool.QueryRow(ctx, `SELECT COUNT(*) FROM push_devices WHERE user_id = $1`, userID).Scan(&count); err != nil {
		t.Fatalf("count push registrations: %v", err)
	}
	if count != MaxPushDevicesPerUser {
		t.Fatalf("push registrations=%d, want %d", count, MaxPushDevicesPerUser)
	}

	devices, err := store.GetPushDevicesForUser(ctx, userID)
	if err != nil || len(devices) == 0 {
		t.Fatalf("load push registrations: count=%d err=%v", len(devices), err)
	}
	updated := devices[0]
	updated.PushToken = "rotated-token-" + suffix
	if err := store.UpsertPushDevice(ctx, updated); err != nil {
		t.Fatalf("update existing registration at cap: %v", err)
	}
	if err := store.DeletePushDevice(ctx, userID, devices[0].DeviceID); err != nil {
		t.Fatalf("delete registration below cap: %v", err)
	}
	if err := store.UpsertPushDevice(ctx, pushDigestTestDevice(userID, suffix+"-replacement")); err != nil {
		t.Fatalf("admit replacement after delete: %v", err)
	}
}

func TestPostgresPushDigestClaimFencingAndExactSnapshot(t *testing.T) {
	store := newTestPostgresStore(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UTC().UnixNano())
	userID := "push-digest-user-" + suffix
	channelID := "push-digest-channel-" + suffix
	t.Cleanup(func() {
		_, _ = store.pool.Exec(context.Background(), `DELETE FROM push_devices WHERE user_id = $1`, userID)
		_, _ = store.pool.Exec(context.Background(), `DELETE FROM notification_channels WHERE id = $1`, channelID)
	})

	device := pushDigestTestDevice(userID, suffix)
	if err := store.UpsertPushDevice(ctx, device); err != nil {
		t.Fatalf("create push device: %v", err)
	}
	devices, err := store.GetPushDevicesForUser(ctx, userID)
	if err != nil || len(devices) != 1 {
		t.Fatalf("load push device: count=%d err=%v", len(devices), err)
	}
	device = devices[0]
	if _, err := store.CreateNotificationChannel(notifications.CreateChannelRequest{
		ID: channelID, Name: "Digest QA", Type: notifications.ChannelTypeAPNs,
	}); err != nil {
		t.Fatalf("create APNs channel: %v", err)
	}

	createdAt := time.Now().UTC().Add(-24*time.Hour - time.Minute).Truncate(time.Microsecond)
	first := PushDigestEnqueue{
		DeviceID: device.ID, ChannelID: channelID,
		DedupeKey: fmt.Sprintf("%064x", 1), Severity: "warning",
		MaintenanceScopeComplete: true,
		WindowSeconds:            86_400,
		CreatedAt:                createdAt,
	}
	result, err := store.EnqueuePushDigestEvents(ctx, []PushDigestEnqueue{first, first})
	if err != nil {
		t.Fatalf("enqueue duplicate digest event: %v", err)
	}
	if result.Inserted != 1 || result.Duplicates != 1 || result.Dropped != 0 {
		t.Fatalf("enqueue result=%+v, want inserted=1 duplicate=1", result)
	}

	claimAt := createdAt.Add(24*time.Hour + time.Second)
	start := make(chan struct{})
	claimResults := make(chan []PushDigestClaim, 2)
	claimErrors := make(chan error, 2)
	var workers sync.WaitGroup
	for range 2 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			claims, claimErr := store.ClaimDuePushDigests(ctx, claimAt, 30*time.Second, 1)
			claimResults <- claims
			claimErrors <- claimErr
		}()
	}
	close(start)
	workers.Wait()
	close(claimResults)
	close(claimErrors)
	for claimErr := range claimErrors {
		if claimErr != nil {
			t.Fatalf("claim due digest: %v", claimErr)
		}
	}
	var firstClaim PushDigestClaim
	var winners int
	for claims := range claimResults {
		if len(claims) == 1 {
			winners++
			firstClaim = claims[0]
		} else if len(claims) != 0 {
			t.Fatalf("claim batch size=%d, want 0 or 1", len(claims))
		}
	}
	if winners != 1 || firstClaim.DeliveryGeneration != 1 || len(firstClaim.Events) != 1 {
		t.Fatalf("claim winners=%d claim=%+v", winners, firstClaim)
	}

	secondCreatedAt := claimAt.Add(time.Second)
	second := PushDigestEnqueue{
		DeviceID: device.ID, ChannelID: channelID,
		DedupeKey: fmt.Sprintf("%064x", 2), Severity: "info",
		MaintenanceScopeComplete: true,
		WindowSeconds:            30,
		CreatedAt:                secondCreatedAt,
	}
	if result, err = store.EnqueuePushDigestEvents(ctx, []PushDigestEnqueue{second}); err != nil || result.Inserted != 1 {
		t.Fatalf("enqueue in-flight successor: result=%+v err=%v", result, err)
	}
	completed, err := store.CompletePushDigestClaim(ctx, device.ID, firstClaim.DeliveryGeneration, []string{firstClaim.Events[0].ID}, claimAt.Add(2*time.Second))
	if err != nil || !completed {
		t.Fatalf("complete exact first snapshot: completed=%t err=%v", completed, err)
	}

	secondClaimAt := secondCreatedAt.Add(31 * time.Second)
	claims, err := store.ClaimDuePushDigests(ctx, secondClaimAt, 30*time.Second, 1)
	if err != nil || len(claims) != 1 || len(claims[0].Events) != 1 {
		t.Fatalf("claim successor snapshot: claims=%+v err=%v", claims, err)
	}
	secondClaim := claims[0]
	claims, err = store.ClaimDuePushDigests(ctx, secondClaimAt.Add(31*time.Second), 30*time.Second, 1)
	if err != nil || len(claims) != 1 {
		t.Fatalf("reclaim expired lease: claims=%+v err=%v", claims, err)
	}
	freshClaim := claims[0]
	if freshClaim.DeliveryGeneration <= secondClaim.DeliveryGeneration {
		t.Fatalf("reclaimed generation=%d, prior=%d", freshClaim.DeliveryGeneration, secondClaim.DeliveryGeneration)
	}
	completed, err = store.CompletePushDigestClaim(ctx, device.ID, secondClaim.DeliveryGeneration, []string{secondClaim.Events[0].ID}, secondClaimAt.Add(32*time.Second))
	if err != nil || completed {
		t.Fatalf("stale completion accepted: completed=%t err=%v", completed, err)
	}
	completed, err = store.CompletePushDigestClaim(ctx, device.ID, freshClaim.DeliveryGeneration, []string{freshClaim.Events[0].ID}, secondClaimAt.Add(33*time.Second))
	if err != nil || !completed {
		t.Fatalf("complete fresh generation: completed=%t err=%v", completed, err)
	}

	var stateCount, eventCount int
	if err := store.pool.QueryRow(ctx, `SELECT COUNT(*) FROM push_alert_digest_states WHERE push_device_id = $1`, device.ID).Scan(&stateCount); err != nil {
		t.Fatalf("count digest states: %v", err)
	}
	if err := store.pool.QueryRow(ctx, `SELECT COUNT(*) FROM push_alert_digest_events WHERE push_device_id = $1`, device.ID).Scan(&eventCount); err != nil {
		t.Fatalf("count digest events: %v", err)
	}
	if stateCount != 0 || eventCount != 0 {
		t.Fatalf("completed digest residue: states=%d events=%d", stateCount, eventCount)
	}
}

func TestPostgresPushDigestClaimSkipsLockedEarliestDevice(t *testing.T) {
	store := newTestPostgresStore(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UTC().UnixNano())
	userID := "push-digest-skip-user-" + suffix
	channelID := "push-digest-skip-channel-" + suffix
	t.Cleanup(func() {
		_, _ = store.pool.Exec(context.Background(), `DELETE FROM push_devices WHERE user_id = $1`, userID)
		_, _ = store.pool.Exec(context.Background(), `DELETE FROM notification_channels WHERE id = $1`, channelID)
	})

	for _, name := range []string{"first", "second"} {
		if err := store.UpsertPushDevice(ctx, pushDigestTestDevice(userID, suffix+"-"+name)); err != nil {
			t.Fatalf("create %s push device: %v", name, err)
		}
	}
	devices, err := store.GetPushDevicesForUser(ctx, userID)
	if err != nil || len(devices) != 2 {
		t.Fatalf("load push devices: count=%d err=%v", len(devices), err)
	}
	byDeviceID := make(map[string]PushDevice, len(devices))
	for _, device := range devices {
		byDeviceID[device.DeviceID] = device
	}
	first := byDeviceID["device-"+suffix+"-first"]
	second := byDeviceID["device-"+suffix+"-second"]
	if first.ID == "" || second.ID == "" {
		t.Fatalf("missing push-device fixtures: %+v", devices)
	}
	if _, err := store.CreateNotificationChannel(notifications.CreateChannelRequest{
		ID: channelID, Name: "Digest SKIP LOCKED QA", Type: notifications.ChannelTypeAPNs,
	}); err != nil {
		t.Fatalf("create APNs channel: %v", err)
	}

	createdAt := time.Now().UTC().Add(-2 * time.Minute).Truncate(time.Microsecond)
	result, err := store.EnqueuePushDigestEvents(ctx, []PushDigestEnqueue{
		{
			DeviceID: first.ID, ChannelID: channelID, DedupeKey: fmt.Sprintf("%064x", 11),
			Severity: "warning", MaintenanceScopeComplete: true, WindowSeconds: 30, CreatedAt: createdAt,
		},
		{
			DeviceID: second.ID, ChannelID: channelID, DedupeKey: fmt.Sprintf("%064x", 12),
			Severity: "warning", MaintenanceScopeComplete: true, WindowSeconds: 30, CreatedAt: createdAt.Add(time.Second),
		},
	})
	if err != nil || result.Inserted != 2 {
		t.Fatalf("enqueue due digests: result=%+v err=%v", result, err)
	}

	lockTx, err := store.pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin lock transaction: %v", err)
	}
	t.Cleanup(func() { _ = lockTx.Rollback(context.Background()) })
	var lockedID string
	if err := lockTx.QueryRow(ctx, `
		SELECT push_device_id
		  FROM push_alert_digest_states
		 WHERE push_device_id = $1
		 FOR UPDATE
	`, first.ID).Scan(&lockedID); err != nil {
		t.Fatalf("lock earliest digest state: %v", err)
	}

	claims, err := store.ClaimDuePushDigests(ctx, time.Now().UTC(), 30*time.Second, 1)
	if err != nil {
		t.Fatalf("claim around locked earliest state: %v", err)
	}
	if len(claims) != 1 || claims[0].Device.ID != second.ID {
		t.Fatalf("claim failed to skip locked earliest state: claims=%+v want device=%s", claims, second.ID)
	}
	if err := lockTx.Rollback(ctx); err != nil {
		t.Fatalf("release earliest state lock: %v", err)
	}
}

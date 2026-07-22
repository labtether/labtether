package alerting

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/notifications"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/securityruntime"
)

const (
	pushDigestClaimLimit  = 50
	pushDigestWorkerLimit = 8
	// A claim leases pushDigestClaimLimit devices at once, while only
	// pushDigestWorkerLimit can send concurrently. Seven worst-case 15-second
	// APNs waves fit inside this lease, preventing another hub from reclaiming a
	// queued device before its local worker starts.
	pushDigestLeaseDuration    = 2 * time.Minute
	pushDigestDeferredInterval = 5 * time.Minute
	pushDigestMaxRetries       = 5
)

// pushDigestStore is intentionally optional. Tests and non-Postgres
// deployments retain immediate APNs fanout, while production Postgres stores
// gain durable aggregation without widening the base push-device contract.
type pushDigestStore interface {
	EnqueuePushDigestEvents(context.Context, []persistence.PushDigestEnqueue) (persistence.PushDigestEnqueueResult, error)
	ClaimDuePushDigests(context.Context, time.Time, time.Duration, int) ([]persistence.PushDigestClaim, error)
	CompletePushDigestClaim(context.Context, string, int64, []string, time.Time) (bool, error)
	ReleasePushDigestClaim(context.Context, string, int64, time.Time, bool) (bool, error)
}

func (d *Deps) durablePushDigestStore() pushDigestStore {
	if d == nil || d.PushDeviceStore == nil {
		return nil
	}
	store, _ := d.PushDeviceStore.(pushDigestStore)
	return store
}

func shouldQueuePushDigest(channelID string, store pushDigestStore, payload map[string]any) bool {
	if store == nil || strings.TrimSpace(channelID) == "" || isIncidentNotificationPayload(payload) {
		return false
	}
	event := strings.ToLower(payloadString(payload, "event"))
	if !strings.HasPrefix(event, "alert.") {
		return false
	}
	severityRank := pushSeverityRank(payloadString(payload, "severity"))
	return severityRank > 0 && severityRank < pushSeverityRank("high")
}

func buildPushDigestEnqueue(device persistence.PushDevice, channelID string, payload map[string]any, now time.Time) (persistence.PushDigestEnqueue, bool) {
	deviceID := strings.TrimSpace(device.ID)
	alertID := firstNonBlank(payloadString(payload, "alert_id"), payloadString(payload, "alert_instance_id"))
	if deviceID == "" || strings.TrimSpace(channelID) == "" || alertID == "" {
		return persistence.PushDigestEnqueue{}, false
	}
	severity := normalizedDigestSeverity(payloadString(payload, "severity"))
	if severity == "" {
		return persistence.PushDigestEnqueue{}, false
	}
	dedupeMaterial := strings.Join([]string{
		strings.ToLower(payloadString(payload, "event")),
		alertID,
		strings.ToLower(payloadString(payload, "state")),
		payloadString(payload, "rule_id"),
	}, "\x00")
	digest := sha256.Sum256([]byte(dedupeMaterial))
	groupSet := notificationPayloadGroupIDs(payload)
	groupIDs := make([]string, 0, len(groupSet))
	for groupID := range groupSet {
		groupIDs = append(groupIDs, groupID)
	}
	sort.Strings(groupIDs)

	windowSeconds := device.DigestWindowSeconds
	if windowSeconds < 30 {
		windowSeconds = 30
	}
	if windowSeconds > 86_400 {
		windowSeconds = 86_400
	}
	return persistence.PushDigestEnqueue{
		DeviceID:                 deviceID,
		ChannelID:                strings.TrimSpace(channelID),
		DedupeKey:                hex.EncodeToString(digest[:]),
		Severity:                 severity,
		NodeOffline:              isNodeOfflinePush(payload),
		ServiceDown:              isServiceDownPush(payload),
		GroupIDs:                 groupIDs,
		MaintenanceScopeComplete: len(groupIDs) <= persistence.MaxPushDigestGroupIDs,
		WindowSeconds:            windowSeconds,
		CreatedAt:                now.UTC(),
	}, true
}

func normalizedDigestSeverity(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "info", "low":
		return "info"
	case "warning", "medium":
		return "warning"
	default:
		return ""
	}
}

// ProcessDuePushDigests claims bounded per-device batches, then delivers them
// through a fixed worker pool. Store leases and generations prevent two hubs
// from fanning out the same snapshot concurrently.
func (d *Deps) ProcessDuePushDigests(ctx context.Context) {
	store := d.durablePushDigestStore()
	if store == nil || d.NotificationStore == nil {
		return
	}
	now := time.Now().UTC()
	claims, err := store.ClaimDuePushDigests(ctx, now, pushDigestLeaseDuration, pushDigestClaimLimit)
	if err != nil {
		log.Printf("notifications: failed to claim due push digests: %v", err)
		return
	}
	if len(claims) == 0 {
		return
	}

	workerCount := pushDigestWorkerLimit
	if len(claims) < workerCount {
		workerCount = len(claims)
	}
	jobs := make(chan persistence.PushDigestClaim)
	var workers sync.WaitGroup
	workers.Add(workerCount)
	for range workerCount {
		go func() {
			defer workers.Done()
			for claim := range jobs {
				d.processPushDigestClaim(ctx, store, claim)
			}
		}()
	}
	for _, claim := range claims {
		select {
		case <-ctx.Done():
			close(jobs)
			workers.Wait()
			return
		case jobs <- claim:
		}
	}
	close(jobs)
	workers.Wait()
}

func (d *Deps) processPushDigestClaim(ctx context.Context, store pushDigestStore, claim persistence.PushDigestClaim) {
	eventIDs := pushDigestEventIDs(claim.Events)
	if len(eventIDs) == 0 {
		_, _ = store.ReleasePushDigestClaim(ctx, claim.Device.ID, claim.DeliveryGeneration, time.Now().UTC().Add(pushDigestDeferredInterval), false)
		return
	}
	now := time.Now().UTC()
	complete := func() {
		if _, err := store.CompletePushDigestClaim(ctx, claim.Device.ID, claim.DeliveryGeneration, eventIDs, time.Now().UTC()); err != nil {
			log.Printf("notifications: failed to complete push digest claim for device %s: %v", safePushDeviceReference(claim.Device.ID), err)
		}
	}
	deferWithoutRetry := func(next time.Time) {
		if _, err := store.ReleasePushDigestClaim(ctx, claim.Device.ID, claim.DeliveryGeneration, next, false); err != nil {
			log.Printf("notifications: failed to defer push digest claim for device %s: %v", safePushDeviceReference(claim.Device.ID), err)
		}
	}

	if !isApplePushPlatform(claim.Device.Platform) || strings.TrimSpace(claim.Device.PushToken) == "" {
		complete()
		return
	}
	if !claim.ExpiresAt.After(now) {
		complete()
		return
	}
	if claim.Device.QuietHoursEnabled {
		timeZone := strings.TrimSpace(claim.Device.TimeZone)
		if _, err := time.LoadLocation(timeZone); timeZone == "" || err != nil {
			// A corrupt/legacy local clock must not violate an explicitly enabled
			// quiet-hours preference. Defer safely until registration is repaired.
			deferWithoutRetry(now.Add(pushDigestDeferredInterval))
			return
		}
		if pushDeviceIsInQuietHours(claim.Device, now) {
			deferWithoutRetry(nextPushQuietHoursEnd(claim.Device, now))
			return
		}
	}

	groupIDs := make(map[string]struct{})
	for _, event := range claim.Events {
		if !event.MaintenanceScopeComplete {
			// The event exceeded the bounded maintenance scope snapshot. Drop it
			// rather than risk leaking an alert through a newly-active window.
			continue
		}
		for _, groupID := range event.GroupIDs {
			if groupID = strings.TrimSpace(groupID); groupID != "" {
				groupIDs[groupID] = struct{}{}
			}
		}
	}
	if d.maintenanceSuppressesGroupIDs(groupIDs) {
		deferWithoutRetry(now.Add(pushDigestDeferredInterval))
		return
	}

	eligible := make([]persistence.PushDigestEvent, 0, len(claim.Events))
	for _, event := range claim.Events {
		if pushDeviceAllowsDigestEvent(claim.Device, event) {
			eligible = append(eligible, event)
		}
	}
	if len(eligible) == 0 {
		complete()
		return
	}

	channel, ok, err := d.getNotificationChannelForRuntime(claim.ChannelID)
	if err != nil {
		d.retryPushDigestClaim(ctx, store, claim, eventIDs, err)
		return
	}
	if !ok || !channel.Enabled || notifications.NormalizeChannelType(channel.Type) != notifications.ChannelTypeAPNs {
		complete()
		return
	}
	adapter := d.NotificationAdapters[notifications.ChannelTypeAPNs]
	if adapter == nil {
		d.retryPushDigestClaim(ctx, store, claim, eventIDs, fmt.Errorf("APNs adapter unavailable"))
		return
	}

	bundleID := firstNonBlank(claim.Device.BundleID, configString(channel.Config, "bundle_id"))
	allowedBundleIDs := allowedAPNsBundleIDs(channel.Config, configString(channel.Config, "bundle_id"))
	if _, allowed := allowedBundleIDs[bundleID]; bundleID == "" || !allowed {
		complete()
		return
	}
	environment := strings.ToLower(strings.TrimSpace(claim.Device.Environment))
	if environment != "sandbox" && environment != "production" {
		if production, _ := channel.Config["production"].(bool); production {
			environment = "production"
		} else {
			environment = "sandbox"
		}
	}

	config := cloneAnyMap(channel.Config)
	config["bundle_id"] = bundleID
	config["production"] = environment == "production"
	config["device_tokens"] = []string{strings.TrimSpace(claim.Device.PushToken)}
	notifications.SetAPNsInvalidDeviceTokenHandler(config, notifications.APNsInvalidDeviceTokenHandler(func(token string) error {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return d.PushDeviceStore.DeletePushDeviceByToken(cleanupCtx, token, bundleID, environment)
	}))

	payload := buildPushDigestPayload(eligible, claim)
	sendCtx, cancel := context.WithTimeout(ctx, notificationDispatchTimeout)
	sendErr := adapter.Send(sendCtx, config, payload)
	cancel()
	if sendErr != nil {
		d.retryPushDigestClaim(ctx, store, claim, eventIDs, sendErr)
		return
	}
	complete()
}

func pushDeviceAllowsDigestEvent(device persistence.PushDevice, event persistence.PushDigestEvent) bool {
	if !event.MaintenanceScopeComplete {
		return false
	}
	severityRank := pushSeverityRank(event.Severity)
	minimumRank := pushSeverityRank(device.MinimumSeverity)
	if minimumRank == 0 {
		minimumRank = pushSeverityRank("warning")
	}
	if severityRank == 0 || severityRank < minimumRank {
		return false
	}
	category := strings.ToLower(strings.TrimSpace(device.PushCategory))
	if category != "all_alerts" && category != "alerts_and_incidents" {
		return false
	}
	if event.NodeOffline && !device.NotifyNodeOffline {
		return false
	}
	if event.ServiceDown && !device.NotifyServiceDown {
		return false
	}
	return true
}

func buildPushDigestPayload(events []persistence.PushDigestEvent, claim persistence.PushDigestClaim) map[string]any {
	count := len(events)
	highestSeverity := "info"
	for _, event := range events {
		if pushSeverityRank(event.Severity) > pushSeverityRank(highestSeverity) {
			highestSeverity = event.Severity
		}
	}
	noun := "alert update is"
	if count != 1 {
		noun = "alert updates are"
	}
	firstEventID := "empty"
	if len(claim.Events) > 0 && strings.TrimSpace(claim.Events[0].ID) != "" {
		firstEventID = strings.TrimSpace(claim.Events[0].ID)
	}
	// A reclaimed lease keeps the same collapse identifier for the same oldest
	// event snapshot, so APNs can replace an ambiguous late duplicate. A later
	// digest starts with a new event ID and therefore gets a new identifier.
	digestIdentity := sha256.Sum256([]byte(strings.TrimSpace(claim.Device.ID) + "\x00" + firstEventID))
	return map[string]any{
		"event":                "alert.digest",
		"title":                "LabTether alert digest",
		"text":                 fmt.Sprintf("%d %s ready to review.", count, noun),
		"summary":              "Non-urgent alert updates were grouped by your notification settings.",
		"severity":             highestSeverity,
		"digest_count":         count,
		"collapse_id":          fmt.Sprintf("digest-%s", hex.EncodeToString(digestIdentity[:12])),
		"apns_priority":        5,
		"apns_expiration_unix": claim.ExpiresAt.UTC().Unix(),
	}
}

func (d *Deps) retryPushDigestClaim(ctx context.Context, store pushDigestStore, claim persistence.PushDigestClaim, eventIDs []string, deliveryErr error) {
	safeError := notifications.SanitizeDeliveryErrorMessage(deliveryErr.Error())
	for _, secret := range []string{claim.Device.PushToken, claim.Device.DeviceID, claim.Device.ID} {
		if secret != "" {
			safeError = strings.ReplaceAll(safeError, secret, "[redacted]")
		}
	}
	nextRetryCount := claim.RetryCount + 1
	nextAttempt := time.Now().UTC().Add(notifications.RetryBackoff(claim.RetryCount))
	if nextRetryCount >= pushDigestMaxRetries || !nextAttempt.Before(claim.ExpiresAt) {
		if _, err := store.CompletePushDigestClaim(ctx, claim.Device.ID, claim.DeliveryGeneration, eventIDs, time.Now().UTC()); err != nil {
			log.Printf("notifications: failed to expire push digest claim for device %s: %v", safePushDeviceReference(claim.Device.ID), err)
		}
		securityruntime.Logf("notifications: push digest delivery exhausted after %d attempts: %s", nextRetryCount, safeError)
		return
	}
	if _, err := store.ReleasePushDigestClaim(ctx, claim.Device.ID, claim.DeliveryGeneration, nextAttempt, true); err != nil {
		log.Printf("notifications: failed to schedule push digest retry for device %s: %v", safePushDeviceReference(claim.Device.ID), err)
	}
}

func pushDigestEventIDs(events []persistence.PushDigestEvent) []string {
	ids := make([]string, 0, len(events))
	seen := make(map[string]struct{}, len(events))
	for _, event := range events {
		id := strings.TrimSpace(event.ID)
		if id == "" {
			continue
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}

func nextPushQuietHoursEnd(device persistence.PushDevice, now time.Time) time.Time {
	location, err := time.LoadLocation(strings.TrimSpace(device.TimeZone))
	if err != nil {
		return now.UTC().Add(pushDigestDeferredInterval)
	}
	local := now.In(location)
	endMinutes := device.QuietHoursEndMinutes
	end := time.Date(local.Year(), local.Month(), local.Day(), endMinutes/60, endMinutes%60, 0, 0, location)
	startMinutes := device.QuietHoursStartMinutes
	currentMinutes := local.Hour()*60 + local.Minute()
	if startMinutes > endMinutes && currentMinutes >= startMinutes {
		end = end.AddDate(0, 0, 1)
	} else if !end.After(local) {
		end = end.AddDate(0, 0, 1)
	}
	return end.Add(time.Minute).UTC()
}

func safePushDeviceReference(deviceID string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(deviceID)))
	return hex.EncodeToString(digest[:6])
}

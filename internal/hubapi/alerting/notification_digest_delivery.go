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
)

const (
	pushDigestClaimLimit  = 50
	pushDigestWorkerLimit = 8
	// The fixed pool can queue claims. Each claim carries its persisted lease;
	// policy reads and delivery must fit that lease, never an estimated wave count.
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
	SettlePushDigestClaim(context.Context, string, int64, persistence.PushDigestSettlement, time.Time) (bool, error)
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
	labels, labelsKnown := notificationAlertLabels(payload["labels"])
	ruleID := payloadString(payload, "rule_id")
	if deviceID == "" || strings.TrimSpace(channelID) == "" || alertID == "" || !labelsKnown || !persistence.ValidPushDigestIdentity(ruleID, labels) {
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
		DeviceID: deviceID,
		RuleID:   ruleID, Labels: labels,
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
	now := d.pushDigestNow()
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
	now := d.pushDigestNow()
	if ctx.Err() != nil || !claim.LeaseExpiresAt.After(now) {
		return
	}
	ctx, cancelClaim := context.WithDeadline(ctx, claim.LeaseExpiresAt)
	defer cancelClaim()
	allIDs := pushDigestEventIDs(claim.Events)
	outcome := persistence.PushDigestSettlement{DeferredUntil: now.Add(pushDigestDeferredInterval)}
	finish := func() { d.settlePushDigest(ctx, store, claim, outcome) }
	if !isApplePushPlatform(claim.Device.Platform) || strings.TrimSpace(claim.Device.PushToken) == "" || !claim.ExpiresAt.After(now) {
		outcome.Discarded = allIDs
		finish()
		return
	}
	if claim.Device.QuietHoursEnabled {
		zone := strings.TrimSpace(claim.Device.TimeZone)
		if _, err := time.LoadLocation(zone); zone == "" || err != nil {
			outcome.Deferred = allIDs
			finish()
			return
		}
		if pushDeviceIsInQuietHours(claim.Device, now) {
			outcome.Deferred = allIDs
			outcome.DeferredUntil = nextPushQuietHoursEnd(claim.Device, now)
			finish()
			return
		}
	}
	// Too little remaining time is a deferral, never a failed APNs attempt.
	if !d.digestCanSend(ctx, claim, now) {
		outcome.Deferred = allIDs
		finish()
		return
	}
	channelCtx, cancelChannel := context.WithTimeout(ctx, pushDigestPolicyBudget)
	channel, ok, channelErr := d.getNotificationChannelForRuntimeContext(channelCtx, claim.ChannelID)
	cancelChannel()
	if channelErr != nil {
		outcome.Deferred = allIDs
		outcome.DeferredUntil = d.pushDigestNow().Add(pushDigestDeferredInterval)
		finish()
		return
	}
	if !d.digestCanSend(ctx, claim, d.pushDigestNow()) {
		outcome.Deferred = allIDs
		outcome.DeferredUntil = d.pushDigestNow().Add(pushDigestDeferredInterval)
		finish()
		return
	}
	adapter := d.NotificationAdapters[notifications.ChannelTypeAPNs]
	// Resolve channel credentials before the final per-entry policy read, so a
	// slow store cannot leave a stale silence decision immediately before Send.
	policyCtx, cancelPolicy := context.WithTimeout(ctx, pushDigestPolicyBudget)
	eligible, snapshotAt := d.selectPushDigestEvents(policyCtx, claim, &outcome)
	policyErr := policyCtx.Err()
	cancelPolicy()
	if policyErr != nil || !d.digestCanSend(ctx, claim, snapshotAt) {
		outcome.Deferred = append(outcome.Deferred, pushDigestEventIDs(eligible)...)
		outcome.DeferredUntil = d.pushDigestNow().Add(pushDigestDeferredInterval)
		finish()
		return
	}
	if len(eligible) == 0 {
		finish()
		return
	}

	if !ok || !channel.Enabled || notifications.NormalizeChannelType(channel.Type) != notifications.ChannelTypeAPNs {
		outcome.Discarded = append(outcome.Discarded, pushDigestEventIDs(eligible)...)
		finish()
		return
	}
	if adapter == nil {
		outcome.Deferred = append(outcome.Deferred, pushDigestEventIDs(eligible)...)
		finish()
		return
	}
	bundleID := firstNonBlank(claim.Device.BundleID, configString(channel.Config, "bundle_id"))
	allowedBundleIDs := allowedAPNsBundleIDs(channel.Config, configString(channel.Config, "bundle_id"))
	if _, allowed := allowedBundleIDs[bundleID]; bundleID == "" || !allowed {
		outcome.Discarded = append(outcome.Discarded, pushDigestEventIDs(eligible)...)
		finish()
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
	sendCtx, cancel := context.WithTimeout(ctx, notificationDispatchTimeout)
	defer cancel()
	notifications.SetAPNsInvalidDeviceTokenHandler(config, notifications.APNsInvalidDeviceTokenHandler(func(token string) error {
		cleanupCtx, cancel := context.WithTimeout(sendCtx, 3*time.Second)
		defer cancel()
		return d.PushDeviceStore.DeletePushDeviceByToken(cleanupCtx, token, bundleID, environment)
	}))

	payload := buildPushDigestPayload(eligible, claim)
	if !d.digestCanSend(ctx, claim, snapshotAt) {
		outcome.Deferred = append(outcome.Deferred, pushDigestEventIDs(eligible)...)
		outcome.DeferredUntil = d.pushDigestNow().Add(pushDigestDeferredInterval)
		finish()
		return
	}
	sendErr := adapter.Send(sendCtx, config, payload)
	cancel()
	if sendErr != nil {
		d.failPushDigestEvents(&outcome, eligible, claim)
	} else {
		outcome.Sent = pushDigestEventIDs(eligible)
	}
	finish()
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
	ids := pushDigestEventIDs(events)
	sort.Strings(ids)
	// Retries of the same sent subset collapse together; deferred entries do
	// not choose the identity and a different subset receives a different ID.
	digestIdentity := sha256.Sum256([]byte(strings.TrimSpace(claim.Device.ID) + "\x00" + strings.Join(ids, "\x00")))
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

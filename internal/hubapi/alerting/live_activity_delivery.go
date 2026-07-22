package alerting

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"log"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/incidents"
	"github.com/labtether/labtether/internal/notifications"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/securityruntime"
)

const (
	liveActivityRetryBatch    = 100
	liveActivitySendTimeout   = 15 * time.Second
	liveActivityRetryInterval = 15 * time.Second
	liveActivityDeliveryLease = 2 * liveActivitySendTimeout
	liveActivityMaxRetryCount = 255
)

type liveActivityAPNsSender interface {
	SendLiveActivity(context.Context, map[string]any, notifications.LiveActivityPush) error
}

// DeliverIncidentLiveActivity updates every current ActivityKit registration
// for one incident. It is intended to run outside the incident HTTP handler.
func (d *Deps) DeliverIncidentLiveActivity(incident incidents.Incident, event string) {
	if d == nil || d.LiveActivityStore == nil || strings.TrimSpace(incident.ID) == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), liveActivitySendTimeout)
	defer cancel()
	now := time.Now().UTC()
	// Queue callbacks are intentionally lightweight and may be accepted out of
	// order by concurrent HTTP handlers. Reloading the committed incident makes
	// each queued job converge on current desired state instead of trusting a
	// stale snapshot or stale terminal event.
	if d.IncidentStore != nil {
		current, found, loadErr := d.IncidentStore.GetIncident(incident.ID)
		if loadErr != nil {
			log.Printf("live activities: failed to reload current incident state: %v", loadErr)
			return
		}
		if found {
			incident = current
		} else {
			// Once an incident is absent, every delayed callback converges on the
			// same privacy-safe terminal tombstone. Never trust an older queued
			// nonterminal snapshot after a hard delete.
			incident.Title = "LabTether Incident"
			incident.Summary = ""
			incident.Assignee = ""
			incident.Status = incidents.StatusClosed
			incident.UpdatedAt = now
			event = "incident.deleted"
		}
	}
	if err := d.LiveActivityStore.DeleteExpiredLiveActivityPushTokens(ctx, now); err != nil {
		log.Printf("live activities: expired-token cleanup failed: %v", err)
	}
	registrations, err := d.LiveActivityStore.ListLiveActivityPushTokensForIncident(ctx, incident.ID, now)
	if err != nil {
		log.Printf("live activities: failed to load incident registrations: %v", err)
		return
	}
	d.deliverLiveActivityRegistrations(ctx, incident, event, registrations, now)
}

// RunLiveActivityRetryLoop retries only the latest material incident state.
// Registration rows contain no plaintext token or state; terminal retry state
// is encrypted with row-bound AAD and expires with the ActivityKit token.
func (d *Deps) RunLiveActivityRetryLoop(ctx context.Context) {
	if d == nil || d.LiveActivityStore == nil {
		return
	}
	// Recover leased work and incident commits missed before the in-memory queue
	// on startup instead of waiting for the first ticker interval.
	d.retryDueLiveActivityPushes(ctx)
	ticker := time.NewTicker(liveActivityRetryInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.retryDueLiveActivityPushes(ctx)
		}
	}
}

func (d *Deps) retryDueLiveActivityPushes(parent context.Context) {
	now := time.Now().UTC()
	ctx, cancel := context.WithTimeout(parent, liveActivitySendTimeout)
	defer cancel()
	if err := d.LiveActivityStore.DeleteExpiredLiveActivityPushTokens(ctx, now); err != nil {
		log.Printf("live activities: expired-token cleanup failed: %v", err)
		return
	}
	due, err := d.LiveActivityStore.ListDueLiveActivityPushTokens(ctx, now, liveActivityRetryBatch)
	if err != nil {
		log.Printf("live activities: failed to list retry work: %v", err)
		return
	}
	byIncident := make(map[string][]persistence.LiveActivityPushToken)
	for _, registration := range due {
		byIncident[registration.IncidentID] = append(byIncident[registration.IncidentID], registration)
	}
	for incidentID, registrations := range byIncident {
		var incident incidents.Incident
		found := false
		if d.IncidentStore != nil {
			var loadErr error
			incident, found, loadErr = d.IncidentStore.GetIncident(incidentID)
			if loadErr != nil {
				log.Printf("live activities: failed to load incident retry state: %v", loadErr)
				continue
			}
		}
		if found {
			d.deliverLiveActivityRegistrations(ctx, incident, "retry", registrations, now)
			continue
		}
		// A hard-deleted incident can still have a pending terminal ActivityKit
		// end. Its minimal final state is encrypted per registration for retry.
		for _, registration := range registrations {
			pendingIncident, decoded := d.decryptPendingLiveActivityIncident(registration)
			if !decoded {
				pendingIncident = incidents.Incident{
					ID: registration.IncidentID, Title: "LabTether Incident",
					Status: incidents.StatusClosed, Severity: incidents.SeverityMedium,
					OpenedAt: now, CreatedAt: now, UpdatedAt: now,
				}
			}
			if incidents.NormalizeStatus(pendingIncident.Status) != incidents.StatusClosed {
				// A missing incident can never be refreshed as open. If the final
				// hard-delete callback was delayed or lost, fail closed by replacing
				// stale pending content with a privacy-safe terminal state.
				pendingIncident.Title = "LabTether Incident"
				pendingIncident.Summary = ""
				pendingIncident.Assignee = ""
				pendingIncident.Status = incidents.StatusClosed
				pendingIncident.UpdatedAt = now
			}
			d.deliverLiveActivityRegistrations(ctx, pendingIncident, "retry", []persistence.LiveActivityPushToken{registration}, now)
		}
	}
	d.reconcileCommittedLiveActivityState(ctx, now)
}

func (d *Deps) reconcileCommittedLiveActivityState(ctx context.Context, now time.Time) {
	registrations, err := d.LiveActivityStore.ListLiveActivityPushTokensForReconciliation(
		ctx, now, liveActivityRetryBatch,
	)
	if err != nil {
		log.Printf("live activities: failed to list reconciliation work: %v", err)
		return
	}
	byIncident := make(map[string][]persistence.LiveActivityPushToken)
	for _, registration := range registrations {
		byIncident[registration.IncidentID] = append(byIncident[registration.IncidentID], registration)
	}
	for incidentID, incidentRegistrations := range byIncident {
		var incident incidents.Incident
		found := false
		if d.IncidentStore != nil {
			var loadErr error
			incident, found, loadErr = d.IncidentStore.GetIncident(incidentID)
			if loadErr != nil {
				log.Printf("live activities: failed to load reconciliation incident: %v", loadErr)
				continue
			}
		}
		if found {
			d.deliverLiveActivityRegistrations(ctx, incident, "incident.reconcile", incidentRegistrations, now)
			continue
		}
		terminal := incidents.Incident{
			ID: incidentID, Title: "LabTether Incident",
			Status: incidents.StatusClosed, Severity: incidents.SeverityMedium,
			OpenedAt: now, CreatedAt: now, UpdatedAt: now,
		}
		d.deliverLiveActivityRegistrations(ctx, terminal, "incident.deleted", incidentRegistrations, now)
	}
}

func (d *Deps) deliverLiveActivityRegistrations(
	ctx context.Context,
	incident incidents.Incident,
	event string,
	registrations []persistence.LiveActivityPushToken,
	now time.Time,
) {
	if len(registrations) == 0 {
		return
	}
	channels, sender := d.liveActivityChannels()

	workerCount := 8
	if len(registrations) < workerCount {
		workerCount = len(registrations)
	}
	jobs := make(chan persistence.LiveActivityPushToken, len(registrations))
	for _, registration := range registrations {
		jobs <- registration
	}
	close(jobs)

	var deliveries sync.WaitGroup
	deliveries.Add(workerCount)
	for range workerCount {
		go func() {
			defer deliveries.Done()
			for registration := range jobs {
				func() {
					defer func() {
						if recovered := recover(); recovered != nil {
							securityruntime.Logf("live activities: delivery panic recovered: %v\n%s", recovered, debug.Stack())
						}
					}()
					d.deliverLiveActivityRegistration(
						ctx, sender, channels, incident, event, registration, now,
					)
				}()
			}
		}()
	}
	deliveries.Wait()
}

func (d *Deps) deliverLiveActivityRegistration(
	ctx context.Context,
	sender liveActivityAPNsSender,
	channels map[string]map[string]any,
	incident incidents.Incident,
	event string,
	registration persistence.LiveActivityPushToken,
	now time.Time,
) {
	if d.NotificationSecrets == nil {
		return
	}
	pendingState, err := d.encryptPendingLiveActivityIncident(registration.ID, incident)
	if err != nil {
		log.Printf("live activities: failed to encrypt desired incident state")
		return
	}
	retryCount := registration.RetryCount
	expectedGeneration := registration.DeliveryGeneration
	desiredIncidentUpdatedAt := incident.UpdatedAt
	if desiredIncidentUpdatedAt.IsZero() {
		desiredIncidentUpdatedAt = now
	}
	if event != "retry" {
		retryCount = 0
		// Direct committed state must supersede any retry that happened to claim
		// an older snapshot between the incident reload and this store write.
		expectedGeneration = -1
	}
	storeCtx, storeCancel := liveActivityStoreWriteContext()
	generation, claimed, claimErr := d.LiveActivityStore.ClaimLiveActivityPushDelivery(
		storeCtx,
		registration.ID,
		expectedGeneration,
		pendingState,
		desiredIncidentUpdatedAt,
		now.Add(liveActivityDeliveryLease),
		retryCount,
	)
	storeCancel()
	if claimErr != nil {
		log.Printf("live activities: failed to claim desired incident state: %v", claimErr)
		return
	}
	if !claimed {
		// Another dispatch/retry already superseded this row snapshot.
		return
	}
	registration.DeliveryGeneration = generation
	registration.RetryCount = retryCount
	registration.PendingStateCiphertext = pendingState

	channelConfig := channels[liveActivityChannelKey(registration.BundleID, registration.Environment)]
	if sender == nil || channelConfig == nil {
		d.scheduleLiveActivityRetry(registration, now)
		return
	}
	plaintext, err := d.NotificationSecrets.DecryptString(
		registration.TokenCiphertext,
		liveActivityTokenAAD(registration.ID),
	)
	if err != nil || !notifications.ValidLiveActivityPushToken(plaintext) || !liveActivityTokenHashMatches(plaintext, registration.TokenHash) {
		// Corrupt, swapped, or undecryptable credentials fail closed and are
		// removed rather than producing a token oracle through repeated sends.
		storeCtx, cancel := liveActivityStoreWriteContext()
		_ = d.LiveActivityStore.DeleteLiveActivityPushTokenByGeneration(storeCtx, registration.ID, generation)
		cancel()
		return
	}
	canMutate := false
	if d.LiveActivityUserCanMutate != nil {
		canMutate = d.LiveActivityUserCanMutate(registration.UserID)
	}
	push := buildIncidentLiveActivityPush(incident, event, registration, plaintext, canMutate, now)
	sendCtx, cancel := context.WithTimeout(ctx, liveActivitySendTimeout)
	err = sender.SendLiveActivity(sendCtx, channelConfig, push)
	cancel()
	if err == nil {
		storeCtx, storeCancel := liveActivityStoreWriteContext()
		defer storeCancel()
		if push.Event == "end" {
			_ = d.LiveActivityStore.DeleteLiveActivityPushTokenByGeneration(storeCtx, registration.ID, generation)
		} else {
			deliveredUpdatedAt := incident.UpdatedAt
			if deliveredUpdatedAt.IsZero() {
				deliveredUpdatedAt = now
			}
			_ = d.LiveActivityStore.ClearLiveActivityPushRetry(
				storeCtx, registration.ID, generation, deliveredUpdatedAt,
			)
		}
		return
	}
	if notifications.IsPermanentAPNsTokenRejection(err) {
		storeCtx, storeCancel := liveActivityStoreWriteContext()
		_ = d.LiveActivityStore.DeleteLiveActivityPushTokenByGeneration(storeCtx, registration.ID, generation)
		storeCancel()
		return
	}
	d.scheduleLiveActivityRetry(registration, now)
}

func (d *Deps) scheduleLiveActivityRetry(
	registration persistence.LiveActivityPushToken,
	now time.Time,
) {
	storeCtx, cancel := liveActivityStoreWriteContext()
	defer cancel()
	nextCount := registration.RetryCount + 1
	if nextCount > liveActivityMaxRetryCount {
		nextCount = liveActivityMaxRetryCount
	}
	next := now.Add(notifications.RetryBackoff(registration.RetryCount))
	if next.After(registration.ExpiresAt) {
		_ = d.LiveActivityStore.DeleteLiveActivityPushTokenByGeneration(
			storeCtx, registration.ID, registration.DeliveryGeneration,
		)
		return
	}
	_ = d.LiveActivityStore.MarkLiveActivityPushRetry(
		storeCtx,
		registration.ID,
		registration.DeliveryGeneration,
		nextCount,
		next,
		registration.PendingStateCiphertext,
	)
}

func liveActivityStoreWriteContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 3*time.Second)
}

func (d *Deps) liveActivityChannels() (map[string]map[string]any, liveActivityAPNsSender) {
	if d.NotificationStore == nil {
		return nil, nil
	}
	adapter := d.NotificationAdapters[notifications.ChannelTypeAPNs]
	sender, ok := adapter.(liveActivityAPNsSender)
	if !ok {
		return nil, nil
	}
	listed, err := d.NotificationStore.ListNotificationChannels(notificationRouteScanLimit)
	if err != nil {
		return nil, nil
	}
	sort.Slice(listed, func(i, j int) bool { return listed[i].ID < listed[j].ID })
	result := make(map[string]map[string]any)
	for _, summary := range listed {
		if !summary.Enabled || notifications.NormalizeChannelType(summary.Type) != notifications.ChannelTypeAPNs {
			continue
		}
		channel, found, loadErr := d.getNotificationChannelForRuntime(summary.ID)
		if loadErr != nil || !found || !channel.Enabled || notifications.NormalizeChannelType(channel.Type) != notifications.ChannelTypeAPNs {
			continue
		}
		bundleID, _ := channel.Config["bundle_id"].(string)
		authKeyPath, _ := channel.Config["auth_key_path"].(string)
		keyID, _ := channel.Config["key_id"].(string)
		teamID, _ := channel.Config["team_id"].(string)
		if strings.TrimSpace(bundleID) == "" || strings.TrimSpace(authKeyPath) == "" ||
			strings.TrimSpace(keyID) == "" || strings.TrimSpace(teamID) == "" {
			continue
		}
		environment := "sandbox"
		if production, _ := channel.Config["production"].(bool); production {
			environment = "production"
		}
		key := liveActivityChannelKey(bundleID, environment)
		if _, alreadySelected := result[key]; !alreadySelected {
			result[key] = cloneAnyMap(channel.Config)
		}
	}
	return result, sender
}

func buildIncidentLiveActivityPush(
	incident incidents.Incident,
	event string,
	registration persistence.LiveActivityPushToken,
	plaintextToken string,
	canMutate bool,
	now time.Time,
) notifications.LiveActivityPush {
	status := incidents.NormalizeStatus(incident.Status)
	if status == "" {
		status = incidents.StatusOpen
	}
	severity := incidents.NormalizeSeverity(incident.Severity)
	if severity == "" {
		severity = incidents.SeverityMedium
	}
	title := boundedIncidentPushText(incident.Title, incidentPushTitleMaxBytes)
	summary := boundedIncidentPushText(incident.Summary, 512)
	assignee := boundedIncidentPushText(incident.Assignee, 96)
	if !registration.ShowFullDetails {
		title = "Incident in progress"
		summary = ""
		assignee = ""
	}
	if title == "" {
		title = "LabTether Incident"
	}
	startedAt := incident.OpenedAt
	if startedAt.IsZero() {
		startedAt = incident.CreatedAt
	}
	if startedAt.IsZero() {
		startedAt = now
	}
	updatedAt := incident.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = now
	}
	pushEvent := "update"
	// Resolved incidents remain updateable because LabTether explicitly supports
	// reopening them. Only closed/hard-deleted incidents end the Activity; an
	// ended ActivityKit token cannot be restarted remotely without push-to-start.
	if status == incidents.StatusClosed {
		pushEvent = "end"
	}
	staleAt := now.Add(20 * time.Minute)
	dismissAt := now
	push := notifications.LiveActivityPush{
		DeviceToken: plaintextToken,
		BundleID:    registration.BundleID,
		ActivityID:  registration.ActivityID,
		Event:       pushEvent,
		Timestamp:   now,
		ExpiresAt:   registration.ExpiresAt,
		Priority:    liveActivityPushPriority(event, pushEvent, severity),
		State: notifications.LiveActivityContentState{
			Title:           title,
			Summary:         summary,
			Status:          status,
			Severity:        severity,
			Assignee:        assignee,
			StartedAt:       startedAt,
			UpdatedAt:       updatedAt,
			ShowFullDetails: registration.ShowFullDetails,
			CanMutate:       canMutate,
		},
	}
	if pushEvent == "end" {
		push.DismissAt = &dismissAt
	} else {
		push.StaleAt = &staleAt
	}
	return push
}

func liveActivityPushPriority(event, pushEvent, severity string) int {
	if pushEvent == "end" || severity == incidents.SeverityCritical {
		return 10
	}
	switch strings.ToLower(strings.TrimSpace(event)) {
	case "incident.updated", "incident.registered":
		return 5
	default:
		return 10
	}
}

func liveActivityTokenHashMatches(token, expectedHash string) bool {
	digest := sha256.Sum256([]byte(token))
	actual := hex.EncodeToString(digest[:])
	return len(actual) == len(expectedHash) && subtle.ConstantTimeCompare([]byte(actual), []byte(expectedHash)) == 1
}

func liveActivityChannelKey(bundleID, environment string) string {
	return strings.TrimSpace(bundleID) + "\x00" + strings.ToLower(strings.TrimSpace(environment))
}

func incidentLiveActivityContentChanged(previous, current incidents.Incident) bool {
	return previous.Title != current.Title ||
		previous.Summary != current.Summary ||
		previous.Status != current.Status ||
		previous.Severity != current.Severity ||
		previous.Assignee != current.Assignee ||
		!previous.OpenedAt.Equal(current.OpenedAt)
}

type pendingLiveActivityIncidentState struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Summary   string    `json:"summary,omitempty"`
	Status    string    `json:"status"`
	Severity  string    `json:"severity"`
	Assignee  string    `json:"assignee,omitempty"`
	OpenedAt  time.Time `json:"opened_at"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (d *Deps) encryptPendingLiveActivityIncident(recordID string, incident incidents.Incident) (string, error) {
	if d.NotificationSecrets == nil {
		return "", errNotificationSecretsUnavailable
	}
	payload, err := json.Marshal(pendingLiveActivityIncidentState{
		ID: incident.ID, Title: incident.Title, Summary: incident.Summary,
		Status: incident.Status, Severity: incident.Severity, Assignee: incident.Assignee,
		OpenedAt: incident.OpenedAt, CreatedAt: incident.CreatedAt, UpdatedAt: incident.UpdatedAt,
	})
	if err != nil {
		return "", err
	}
	return d.NotificationSecrets.EncryptString(string(payload), pendingLiveActivityStateAAD(recordID))
}

func (d *Deps) decryptPendingLiveActivityIncident(registration persistence.LiveActivityPushToken) (incidents.Incident, bool) {
	if d.NotificationSecrets == nil || strings.TrimSpace(registration.PendingStateCiphertext) == "" {
		return incidents.Incident{}, false
	}
	plaintext, err := d.NotificationSecrets.DecryptString(
		registration.PendingStateCiphertext,
		pendingLiveActivityStateAAD(registration.ID),
	)
	if err != nil {
		return incidents.Incident{}, false
	}
	var state pendingLiveActivityIncidentState
	if err := json.Unmarshal([]byte(plaintext), &state); err != nil || strings.TrimSpace(state.ID) == "" {
		return incidents.Incident{}, false
	}
	return incidents.Incident{
		ID: state.ID, Title: state.Title, Summary: state.Summary,
		Status: state.Status, Severity: state.Severity, Assignee: state.Assignee,
		OpenedAt: state.OpenedAt, CreatedAt: state.CreatedAt, UpdatedAt: state.UpdatedAt,
	}, true
}

func pendingLiveActivityStateAAD(recordID string) string {
	return "live-activity-pending-state:" + recordID
}

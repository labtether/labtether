package alerting

import (
	"context"
	"log"
	"time"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/notifications"
	"github.com/labtether/labtether/internal/persistence"
)

const pushDigestPolicyBudget = 5 * time.Second
const pushDigestSettleBudget = 5 * time.Second

func (d *Deps) pushDigestNow() time.Time {
	if d.pushDigestClock != nil {
		return d.pushDigestClock().UTC()
	}
	return time.Now().UTC()
}

func (d *Deps) digestCanSend(ctx context.Context, claim persistence.PushDigestClaim, snapshotAt time.Time) bool {
	now := d.pushDigestNow()
	if ctx.Err() != nil || snapshotAt.IsZero() || now.Before(snapshotAt) || now.Sub(snapshotAt) >= pushDigestPolicyBudget {
		return false
	}
	reserve := notificationDispatchTimeout + pushDigestSettleBudget
	if claim.LeaseExpiresAt.Sub(now) < reserve {
		return false
	}
	if deadline, ok := ctx.Deadline(); ok && deadline.Sub(now) < reserve {
		return false
	}
	return true
}

func (d *Deps) selectPushDigestEvents(ctx context.Context, claim persistence.PushDigestClaim, outcome *persistence.PushDigestSettlement) ([]persistence.PushDigestEvent, time.Time) {
	snapshotAt := d.pushDigestNow()
	var silences []alerts.AlertSilence
	store, known := d.AlertInstanceStore.(interface {
		ListAlertSilencesContext(context.Context, int, bool) ([]alerts.AlertSilence, error)
	})
	if known {
		var err error
		silences, err = store.ListAlertSilencesContext(ctx, notificationSilenceScanLimit, true)
		known = err == nil && len(silences) < notificationSilenceScanLimit
	}
	// A group has one result per claim, even when every entry shares it. The
	// entire snapshot, including all unique groups, shares one five-second bound.
	groups := make(map[string]bool)
	eligible := make([]persistence.PushDigestEvent, 0, len(claim.Events))
	for _, event := range claim.Events {
		now := d.pushDigestNow()
		if !event.ExpiresAt.IsZero() && !event.ExpiresAt.After(now) {
			outcome.Discarded = append(outcome.Discarded, event.ID)
			continue
		}
		if !pushDeviceAllowsDigestEvent(claim.Device, event) {
			outcome.Discarded = append(outcome.Discarded, event.ID)
			continue
		}
		blocked := !known || ctx.Err() != nil || now.Before(snapshotAt) || now.Sub(snapshotAt) >= pushDigestPolicyBudget || !persistence.ValidPushDigestIdentity(event.RuleID, event.Labels)
		if !blocked {
			rule := alerts.Rule{ID: event.RuleID, Labels: event.Labels}
			for _, silence := range silences {
				if matchesSilence(rule, silence) {
					blocked = true
					break
				}
			}
		}
		if !blocked {
			for _, id := range event.GroupIDs {
				suppressed, loaded := groups[id]
				if !loaded {
					suppressed = true
					if ctx.Err() == nil && d.pushDigestNow().Sub(snapshotAt) < pushDigestPolicyBudget && d.EvaluateGuardrailsContext != nil {
						guardrails, err := d.EvaluateGuardrailsContext(ctx, id, snapshotAt)
						suppressed = err != nil || guardrails.SuppressAlerts
					}
					groups[id] = suppressed
				}
				if suppressed {
					blocked = true
					break
				}
			}
		}
		if blocked {
			outcome.Deferred = append(outcome.Deferred, event.ID)
		} else {
			eligible = append(eligible, event)
		}
	}
	return eligible, snapshotAt
}

func (d *Deps) failPushDigestEvents(outcome *persistence.PushDigestSettlement, events []persistence.PushDigestEvent, claim persistence.PushDigestClaim) {
	now := d.pushDigestNow()
	outcome.RetryAt = make(map[string]time.Time)
	for _, event := range events {
		expiry := event.ExpiresAt
		if expiry.IsZero() {
			expiry = claim.ExpiresAt
		}
		next := now.Add(notifications.RetryBackoff(event.RetryCount))
		if event.RetryCount+1 >= pushDigestMaxRetries || !next.Before(expiry) {
			outcome.Discarded = append(outcome.Discarded, event.ID)
		} else {
			outcome.RetryAt[event.ID] = next
		}
	}
}

func (d *Deps) settlePushDigest(ctx context.Context, store pushDigestStore, claim persistence.PushDigestClaim, outcome persistence.PushDigestSettlement) {
	now := d.pushDigestNow()
	if ctx.Err() != nil || !claim.LeaseExpiresAt.After(now) {
		return
	}
	bounded, cancel := context.WithTimeout(ctx, pushDigestSettleBudget)
	defer cancel()
	if _, err := store.SettlePushDigestClaim(bounded, claim.Device.ID, claim.DeliveryGeneration, outcome, now); err != nil {
		log.Printf("notifications: failed to settle push digest claim for device %s", safePushDeviceReference(claim.Device.ID))
	}
}

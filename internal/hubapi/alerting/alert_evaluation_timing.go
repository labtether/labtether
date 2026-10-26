package alerting

import (
	"context"
	"log"
	"time"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/persistence"
)

func (d *Deps) claimAutomaticAlertAttempt(ctx context.Context, ruleID string, at time.Time) bool {
	store, ok := d.AlertStore.(persistence.AlertRuleTimingStore)
	if !ok {
		return false
	}
	readCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	claimed, err := store.ClaimAlertRuleEvaluation(readCtx, ruleID, at)
	if err != nil {
		log.Printf("alert evaluator: automatic timing claim failed for %s: %v", ruleID, err)
		return false
	}
	return claimed
}

func (d *Deps) alertFingerprintMayFire(ctx context.Context, rule alerts.Rule, fingerprint string, at time.Time) bool {
	store, ok := d.AlertInstanceStore.(persistence.AlertInstanceTimingStore)
	if !ok {
		return false
	}
	readCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	history, err := store.AlertFingerprintTiming(readCtx, rule.ID, fingerprint)
	if err != nil {
		log.Printf("alert evaluator: firing history unavailable for %s: %v", rule.ID, err)
		return false
	}
	for _, hold := range []struct {
		anchor  *time.Time
		seconds int
	}{
		{history.LastFirstFiredAt, rule.CooldownSeconds},
		{history.LastResolvedAt, rule.ReopenAfterSeconds},
	} {
		if hold.seconds < 0 || hold.seconds > alerts.MaxDurationSeconds {
			return false
		}
		if hold.anchor != nil && at.Before(hold.anchor.Add(time.Duration(hold.seconds)*time.Second)) {
			return false
		}
	}
	return readCtx.Err() == nil && !at.IsZero()
}

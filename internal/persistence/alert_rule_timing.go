package persistence

import (
	"context"
	"time"
)

// Automatic admission is separate from manual evaluation history. This claims
// an attempt, not a long-running distributed execution lease.
type AlertRuleTimingStore interface {
	ClaimAlertRuleEvaluation(context.Context, string, time.Time) (bool, error)
}

// Only known pending-to-firing transitions supply these anchors. NULL legacy
// history and never-fired placeholders deliberately do not impose a delay.
type AlertFingerprintTiming struct {
	LastFirstFiredAt *time.Time
	LastResolvedAt   *time.Time
}

type AlertInstanceTimingStore interface {
	AlertFingerprintTiming(context.Context, string, string) (AlertFingerprintTiming, error)
}

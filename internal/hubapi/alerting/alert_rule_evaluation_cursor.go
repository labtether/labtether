package alerting

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/persistence"
)

// Suppression has its own parent-bound read budget. The work slice covers page,
// condition and state-store operations; it does not own asynchronous delivery.
func (d *Deps) evaluateAlertRuleSlice(ctx context.Context, now func() time.Time) {
	store, ok := d.AlertStore.(persistence.AlertRuleEvaluationPageStore)
	if !ok || d.AlertInstanceStore == nil || !d.alertRuleEvaluationMu.TryLock() {
		return
	}
	defer d.alertRuleEvaluationMu.Unlock()
	if ctx.Err() != nil {
		return
	}
	// Keep suppression outside the page slice so a slow successful prefetch does
	// not consume every tick before the cursor can advance.
	prefetch, err := d.prefetchAlertRuleSlice(ctx)
	if err != nil {
		log.Printf("alert evaluator: prefetch failed: %v", err)
		return
	}
	if ctx.Err() != nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for ctx.Err() == nil {
		cursor := d.alertRuleEvaluationCursor
		page, err := store.PageActiveAlertRules(ctx, cursor, persistence.AlertRuleEvaluationPageSize)
		if err != nil {
			log.Printf("alert evaluator: failed to page active rules: %v", err)
			return
		}
		if !validAlertRuleEvaluationPage(cursor, page) {
			log.Printf("alert evaluator: invalid active rule page")
			return
		}
		d.alertRuleEvaluationCursor.Started = true
		d.alertRuleEvaluationCursor.ThroughID = page.ThroughID
		// Composite conditions use the complete exact-rule snapshot owner.
		// Optional global firing prefetch must not consume this slice before
		// any cursor progress is made.
		for _, rule := range page.Rules {
			if ctx.Err() != nil {
				return
			}
			at := now().UTC()
			if ctx.Err() != nil {
				return
			}
			if d.claimAutomaticAlertAttempt(ctx, rule.ID, at) {
				d.evaluateSingleRuleAt(ctx, rule, prefetch, at)
			}
			// A failed/not-due consideration is retried next sweep. It must not
			// indefinitely hide later IDs; unconsidered rows retain their position.
			d.alertRuleEvaluationCursor.AfterID = rule.ID
		}
		if page.Done {
			d.alertRuleEvaluationCursor = persistence.AlertRuleEvaluationCursor{}
			return // Never start a second sweep in the same tick.
		}
	}
}

func validAlertRuleEvaluationPage(cursor persistence.AlertRuleEvaluationCursor, page persistence.AlertRuleEvaluationPage) bool {
	if len(page.Rules) > persistence.AlertRuleEvaluationPageSize || (!page.Done && len(page.Rules) == 0) ||
		(cursor.Started && page.ThroughID != cursor.ThroughID) || page.ThroughID < cursor.AfterID {
		return false
	}
	previous := cursor.AfterID
	for _, rule := range page.Rules {
		if rule.ID <= previous || rule.ID > page.ThroughID || rule.Status != alerts.RuleStatusActive {
			return false
		}
		previous = rule.ID
	}
	return true
}

func (d *Deps) prefetchAlertRuleSlice(parent context.Context) (*AlertEvaluationPrefetch, error) {
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	prefetch := &AlertEvaluationPrefetch{assetSnapshot: &alertAssetSnapshot{}}
	// Resolve asset targets only when a condition needs them. Exact IDs avoid
	// copying the fleet; broad/group/selector rules share one complete snapshot.
	// The snapshot is confined to this slice and retains its fleet-sized cost.
	var err error
	prefetch.Suppression, err = d.newAlertSuppressionPrefetch(ctx)
	if err != nil {
		return nil, err
	}
	return prefetch, ctx.Err()
}

type alertAssetSnapshot struct {
	loaded bool
	assets []assets.Asset
	err    error
}

func (d *Deps) prefetchedRuleAssets(ctx context.Context, rule alerts.Rule, prefetch *AlertEvaluationPrefetch) ([]assets.Asset, error) {
	snapshot := prefetch.assetSnapshot
	if snapshot == nil {
		return prefetch.Assets, nil // Preserve explicit caller-provided prefetches.
	}
	switch rule.Kind {
	case alerts.RuleKindHeartbeatStale, alerts.RuleKindMetricThreshold, alerts.RuleKindMetricDeadman:
	case alerts.RuleKindLogPattern:
		if alerts.NormalizeTargetScope(rule.TargetScope) == alerts.TargetScopeGlobal && len(rule.Targets) == 0 {
			return nil, nil // Global logs supply their own exact asset witnesses.
		}
	default:
		return nil, nil
	}
	if !alertTargetsNeedInventory(rule, true) || d.AssetStore == nil {
		return nil, nil // Even a loaded snapshot must not reach exact-ID rules.
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !snapshot.loaded {
		snapshot.loaded = true
		store, ok := d.AssetStore.(persistence.AlertAssetContextStore)
		if !ok {
			snapshot.err = fmt.Errorf("context-aware alert asset inventory unavailable")
		} else {
			snapshot.assets, snapshot.err = store.ListAssetsContext(ctx)
			if snapshot.err == nil {
				snapshot.err = ctx.Err()
			}
			if snapshot.err == nil && snapshot.assets == nil {
				snapshot.assets = []assets.Asset{} // A complete empty inventory is cached too.
			}
		}
	}
	if snapshot.err != nil {
		return nil, snapshot.err
	}
	return snapshot.assets, nil
}

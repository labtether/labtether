package alerting

import (
	"context"
	"time"

	"github.com/labtether/labtether/internal/alerts"
)

// pausedRuleResolveTimeout bounds how long a pause request spends closing
// alerts. It stays under the Hub's 60 s response write timeout.
const pausedRuleResolveTimeout = 45 * time.Second

// resolvePausedRuleAlerts closes every open alert of a rule that was just
// paused. The evaluator skips paused rules, so anything left open here would
// stay open for good. Each pass resolves up to 100 alerts per status; passes
// repeat until one resolves nothing.
func (d *Deps) resolvePausedRuleAlerts(rule alerts.Rule) {
	ctx, cancel := context.WithTimeout(context.Background(), pausedRuleResolveTimeout)
	defer cancel()
	for ctx.Err() == nil {
		if d.resolveStaleInstancesContext(ctx, rule, nil) == 0 {
			return
		}
	}
}

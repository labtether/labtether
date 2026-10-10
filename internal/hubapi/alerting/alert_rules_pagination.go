package alerting

import (
	"context"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/persistence"
)

// listAccessibleAlertRules applies authorization before the caller's offset.
// Store pages may contain no visible rules even when later pages do.
func (d *Deps) listAccessibleAlertRules(ctx context.Context, filter persistence.AlertRuleFilter, groupAccess map[string]struct{}) ([]alerts.Rule, error) {
	const batchSize = 500
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > batchSize {
		limit = batchSize
	}
	page := make([]alerts.Rule, 0, limit)
	storeFilter := filter
	storeFilter.Limit = batchSize
	visible := 0
	for storeOffset := 0; ; {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		storeFilter.Offset = storeOffset
		batch, err := d.AlertStore.ListAlertRules(storeFilter)
		if err != nil {
			return nil, err
		}
		for _, rule := range batch {
			if !alertRuleAllowed(ctx, rule, groupAccess) {
				continue
			}
			if visible >= filter.Offset {
				page = append(page, rule)
				if len(page) == limit {
					return page, nil
				}
			}
			visible++
		}
		if len(batch) < batchSize {
			return page, nil
		}
		storeOffset += len(batch)
	}
}

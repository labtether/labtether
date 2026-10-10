package alerting

import (
	"context"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/incidents"
	"github.com/labtether/labtether/internal/persistence"
)

// Store offsets count every record. Restricted offsets must count only records
// the caller can see, even when an entire store batch is hidden.
func (d *Deps) listAccessibleAlertInstances(ctx context.Context, filter persistence.AlertInstanceFilter, groupAccess map[string]struct{}) ([]alerts.AlertInstance, error) {
	const batchSize = 500
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > batchSize {
		limit = batchSize
	}
	page := make([]alerts.AlertInstance, 0)
	storeFilter := filter
	storeFilter.Limit = batchSize
	visible := 0
	for storeOffset := 0; ; {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		storeFilter.Offset = storeOffset
		batch, err := d.AlertInstanceStore.ListAlertInstances(storeFilter)
		if err != nil {
			return nil, err
		}
		for _, instance := range batch {
			allowed, err := d.alertInstanceAllowed(ctx, instance, groupAccess)
			if err != nil {
				return nil, err
			}
			if !allowed {
				continue
			}
			if visible >= filter.Offset {
				page = append(page, instance)
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

func (d *Deps) listAccessibleIncidents(ctx context.Context, filter persistence.IncidentFilter, groupAccess map[string]struct{}) ([]incidents.Incident, error) {
	const batchSize = 500
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > batchSize {
		limit = batchSize
	}
	page := make([]incidents.Incident, 0)
	storeFilter := filter
	storeFilter.Limit = batchSize
	visible := 0
	for storeOffset := 0; ; {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		storeFilter.Offset = storeOffset
		batch, err := d.IncidentStore.ListIncidents(storeFilter)
		if err != nil {
			return nil, err
		}
		for _, incident := range batch {
			allowed, err := d.incidentAllowed(ctx, incident, groupAccess)
			if err != nil {
				return nil, err
			}
			if !allowed {
				continue
			}
			if visible >= filter.Offset {
				page = append(page, incident)
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

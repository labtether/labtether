package shared

import (
	"fmt"
	"strings"

	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/persistence"
)

// ResolveEnabledHubCollector never substitutes a collector for a saved owner.
// Legacy unowned callers require a complete, type-filtered uniqueness check.
func ResolveEnabledHubCollector(store persistence.HubCollectorStore, collectorType, collectorID string) (*hubcollector.Collector, error) {
	if store == nil {
		return nil, fmt.Errorf("hub collector store unavailable")
	}
	collectorType = hubcollector.NormalizeCollectorType(collectorType)
	if collectorType == "" {
		return nil, fmt.Errorf("invalid collector type")
	}
	collectorID = strings.TrimSpace(collectorID)
	var selected hubcollector.Collector
	if collectorID != "" {
		collector, found, err := store.GetHubCollector(collectorID)
		if err != nil {
			return nil, fmt.Errorf("failed to load hub collector: %w", err)
		}
		if !found || collector.ID != collectorID {
			return nil, fmt.Errorf("no active %s collector configured", collectorType)
		}
		selected = collector
	} else {
		lookup, ok := store.(persistence.EnabledHubCollectorLookup)
		if !ok {
			return nil, fmt.Errorf("bounded hub collector lookup unavailable")
		}
		collectors, err := lookup.FindEnabledHubCollectorsByType(collectorType)
		if err != nil {
			return nil, fmt.Errorf("failed to list hub collectors: %w", err)
		}
		if len(collectors) == 0 {
			return nil, fmt.Errorf("no active %s collector configured", collectorType)
		}
		if len(collectors) != 1 {
			return nil, fmt.Errorf("collector_id is required when multiple active %s collectors are configured", collectorType)
		}
		selected = collectors[0]
	}
	if strings.TrimSpace(selected.ID) == "" || !selected.Enabled || hubcollector.NormalizeCollectorType(selected.CollectorType) != collectorType {
		return nil, fmt.Errorf("no active %s collector configured", collectorType)
	}
	return &selected, nil
}

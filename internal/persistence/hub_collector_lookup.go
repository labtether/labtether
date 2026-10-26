package persistence

import (
	"fmt"

	"github.com/labtether/labtether/internal/hubcollector"
)

// EnabledHubCollectorLookup resolves legacy singleton runtime ownership without
// a global inventory page. Two matches are enough to reject an ambiguous owner.
type EnabledHubCollectorLookup interface {
	FindEnabledHubCollectorsByType(collectorType string) ([]hubcollector.Collector, error)
}

func (s *PostgresStore) FindEnabledHubCollectorsByType(collectorType string) ([]hubcollector.Collector, error) {
	collectorType = hubcollector.NormalizeCollectorType(collectorType)
	if collectorType == "" {
		return nil, fmt.Errorf("invalid collector type")
	}
	ctx, cancel := agentLifecycleStoreContext()
	defer cancel()
	rows, err := s.pool.Query(ctx,
		`SELECT `+hubCollectorColumns+` FROM hub_collectors
		 WHERE collector_type = $1 AND enabled = TRUE ORDER BY id LIMIT 2`, collectorType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]hubcollector.Collector, 0, 2)
	for rows.Next() {
		collector, err := scanHubCollector(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, collector)
	}
	return out, rows.Err()
}

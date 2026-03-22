package groupfeatures

import (
	"context"
	"log"
	"time"

	"github.com/labtether/labtether/internal/persistence"
)

// RunReliabilityMaterializer starts the background reliability materializer loop.
// It ticks every hour, computing and persisting reliability records for all groups.
//
// NOTE: As of the current codebase this function is not called from any startup
// path — it is preserved here for completeness and future activation.
func (d *Deps) RunReliabilityMaterializer(ctx context.Context, store persistence.ReliabilityHistoryStore) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	log.Printf("reliability materializer started (interval=1h)")

	// Run once at startup after a short delay.
	select {
	case <-ctx.Done():
		return
	case <-time.After(30 * time.Second):
		d.MaterializeReliability(store)
	}

	for {
		select {
		case <-ctx.Done():
			log.Printf("reliability materializer stopped")
			return
		case <-ticker.C:
			d.MaterializeReliability(store)
		}
	}
}

// MaterializeReliability computes current reliability for all groups and writes
// the results to the ReliabilityHistoryStore. It is exported so callers can
// trigger a materialisation on demand.
func (d *Deps) MaterializeReliability(store persistence.ReliabilityHistoryStore) {
	if d.GroupStore == nil || store == nil {
		return
	}

	groupList, err := d.GroupStore.ListGroups()
	if err != nil {
		log.Printf("reliability materializer: failed to list groups: %v", err)
		return
	}

	now := time.Now().UTC()
	from := now.Add(-24 * time.Hour)

	for _, group := range groupList {
		record, err := d.BuildGroupReliabilityRecord(group, from, now)
		if err != nil {
			log.Printf("reliability materializer: failed to compute for group %s: %v", group.ID, err)
			continue
		}

		factors := map[string]any{
			"assets_total":   record.AssetsTotal,
			"assets_online":  record.AssetsOnline,
			"assets_stale":   record.AssetsStale,
			"assets_offline": record.AssetsOffline,
			"failed_actions": record.FailedActions,
			"failed_updates": record.FailedUpdates,
			"error_logs":     record.ErrorLogs,
			"dead_letters":   record.DeadLetters,
		}

		if err := store.InsertReliabilityRecord(group.ID, record.Score, record.Grade, factors, 24); err != nil {
			log.Printf("reliability materializer: failed to insert record for group %s: %v", group.ID, err)
		}
	}

	// Prune old records (> 90 days).
	pruned, err := store.PruneReliabilityHistory(90)
	if err != nil {
		log.Printf("reliability materializer: failed to prune old records: %v", err)
	} else if pruned > 0 {
		log.Printf("reliability materializer: pruned %d old records", pruned)
	}
}

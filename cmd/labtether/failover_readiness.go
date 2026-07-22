package main

import (
	"context"
	"log"
	"time"

	resourcespkg "github.com/labtether/labtether/internal/hubapi/resources"
)

// Type alias so that call-sites within this package are unchanged.
type failoverReadinessSnapshot = resourcespkg.FailoverReadinessSnapshot

func (s *apiServer) runFailoverReadinessChecker(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	log.Printf("failover readiness checker started (interval=1h)")
	s.checkAllFailoverReadiness(ctx)

	for {
		select {
		case <-ctx.Done():
			log.Printf("failover readiness checker stopped")
			return
		case <-ticker.C:
			s.checkAllFailoverReadiness(ctx)
		}
	}
}

func (s *apiServer) checkAllFailoverReadiness(ctx context.Context) {
	if s.failoverStore == nil {
		return
	}

	pairs, err := s.failoverStore.ListFailoverPairs(500)
	if err != nil {
		log.Printf("failover readiness: failed to list pairs: %v", err)
		return
	}
	if len(pairs) == 0 {
		return
	}

	snapshot, err := s.buildFailoverReadinessSnapshot()
	if err != nil {
		log.Printf("failover readiness: skipped run because the inventory snapshot is unavailable: %v", err)
		return
	}

	now := time.Now().UTC()
	for _, pair := range pairs {
		select {
		case <-ctx.Done():
			return
		default:
		}

		score := resourcespkg.ComputeFailoverReadiness(pair, snapshot)
		if err := s.failoverStore.UpdateFailoverReadiness(pair.ID, score, now); err != nil {
			log.Printf("failover readiness: failed to update pair %s: %v", pair.ID, err)
		}
	}
}

func (s *apiServer) buildFailoverReadinessSnapshot() (failoverReadinessSnapshot, error) {
	return resourcespkg.LoadFailoverReadinessSnapshot(s.groupStore, s.assetStore)
}

package main

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/groups"
	resourcespkg "github.com/labtether/labtether/internal/hubapi/resources"
)

// Type aliases so that call-sites within this package are unchanged.
type failoverReadinessAssetCounts = resourcespkg.FailoverReadinessAssetCounts
type failoverReadinessSnapshot = resourcespkg.FailoverReadinessSnapshot

func (s *apiServer) runFailoverReadinessChecker(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	log.Printf("failover readiness checker started (interval=1h)")

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

	snapshot := s.buildFailoverReadinessSnapshot()
	if !snapshot.GroupsLoaded {
		log.Printf("failover readiness: skipped run because group snapshot is unavailable")
		return
	}

	for _, pair := range pairs {
		select {
		case <-ctx.Done():
			return
		default:
		}

		score := resourcespkg.ComputeFailoverReadiness(pair, snapshot)
		now := time.Now().UTC()
		if err := s.failoverStore.UpdateFailoverReadiness(pair.ID, score, now); err != nil {
			log.Printf("failover readiness: failed to update pair %s: %v", pair.ID, err)
		}
	}
}

func (s *apiServer) buildFailoverReadinessSnapshot() failoverReadinessSnapshot {
	snapshot := failoverReadinessSnapshot{
		GroupsByID:         map[string]groups.Group{},
		GroupsLoaded:       false,
		AssetCountsByGroup: map[string]failoverReadinessAssetCounts{},
		AssetsLoaded:       false,
	}
	if s.groupStore == nil {
		return snapshot
	}

	groupList, err := s.groupStore.ListGroups()
	if err != nil {
		log.Printf("failover readiness: failed to list groups: %v", err)
		return snapshot
	}
	snapshot.GroupsByID = make(map[string]groups.Group, len(groupList))
	for _, groupEntry := range groupList {
		groupID := strings.TrimSpace(groupEntry.ID)
		if groupID == "" {
			continue
		}
		snapshot.GroupsByID[groupID] = groupEntry
	}
	snapshot.GroupsLoaded = true

	if s.assetStore == nil {
		return snapshot
	}

	assetList, err := s.assetStore.ListAssets()
	if err != nil {
		log.Printf("failover readiness: failed to list assets: %v", err)
		return snapshot
	}
	snapshot.AssetsLoaded = true
	snapshot.AssetCountsByGroup = resourcespkg.FailoverAssetCountsByGroup(assetList)
	return snapshot
}

package main

import (
	"context"
	"log"
	"sync"
	"time"

	alertingpkg "github.com/labtether/labtether/internal/hubapi/alerting"
	"github.com/labtether/labtether/internal/synthetic"
)

func (s *apiServer) runSyntheticRunner(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	log.Printf("synthetic runner started (poll=15s)")

	for {
		select {
		case <-ctx.Done():
			log.Printf("synthetic runner stopped")
			return
		case <-ticker.C:
			s.runPendingSyntheticChecks(ctx)
		}
	}
}

func (s *apiServer) runPendingSyntheticChecks(ctx context.Context) {
	if s.syntheticStore == nil {
		return
	}

	checks, err := s.syntheticStore.ListSyntheticChecks(500, true)
	if err != nil {
		log.Printf("synthetic runner: failed to list checks: %v", err)
		return
	}

	now := time.Now().UTC()

	// Run checks concurrently with a bounded semaphore so that up to 500 checks
	// don't all execute serially. pgxpool handles concurrent DB calls safely.
	const maxConcurrentChecks = 10
	sem := make(chan struct{}, maxConcurrentChecks)
	var wg sync.WaitGroup

checkLoop:
	for _, check := range checks {
		select {
		case <-ctx.Done():
			break checkLoop
		default:
		}

		// Check if enough time has elapsed since last run.
		if check.LastRunAt != nil {
			interval := time.Duration(check.IntervalSeconds) * time.Second
			if now.Sub(*check.LastRunAt) < interval {
				continue
			}
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(check synthetic.Check) {
			defer wg.Done()
			defer func() { <-sem }()

			result := alertingpkg.ExecuteSyntheticCheck(check)
			recorded, err := s.syntheticStore.RecordSyntheticResult(check.ID, result)
			if err != nil {
				log.Printf("synthetic runner: failed to record result for check %s: %v", check.ID, err)
				return
			}
			if err := s.syntheticStore.UpdateSyntheticCheckStatus(check.ID, recorded.Status, recorded.CheckedAt); err != nil {
				log.Printf("synthetic runner: failed to update check status for %s: %v", check.ID, err)
			}
		}(check)
	}

	wg.Wait()
}

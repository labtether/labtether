package main

import (
	"context"
	"log"
	"sync"
	"time"

	alertingpkg "github.com/labtether/labtether/internal/hubapi/alerting"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/synthetic"
)

const syntheticRunnerBatchLimit = 500

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

	now := time.Now().UTC()
	checks, err := s.dueSyntheticChecks(ctx, now, syntheticRunnerBatchLimit)
	if err != nil {
		log.Printf("synthetic runner: failed to list checks: %v", err)
		return
	}

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

		if check.IntervalSeconds <= 0 {
			continue
		}
		if _, running := s.syntheticCheckRunState.LoadOrStore(check.ID, struct{}{}); running {
			continue
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(check synthetic.Check) {
			defer wg.Done()
			defer func() { <-sem }()
			defer s.syntheticCheckRunState.Delete(check.ID)

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

func (s *apiServer) dueSyntheticChecks(ctx context.Context, now time.Time, limit int) ([]synthetic.Check, error) {
	if dueStore, ok := s.syntheticStore.(persistence.DueSyntheticCheckStore); ok {
		return dueStore.ListDueSyntheticChecks(ctx, now, limit)
	}

	checks, err := s.syntheticStore.ListSyntheticChecks(limit, true)
	if err != nil {
		return nil, err
	}

	due := make([]synthetic.Check, 0, len(checks))
	for _, check := range checks {
		if check.IntervalSeconds <= 0 {
			continue
		}
		if check.LastRunAt != nil && now.Sub(*check.LastRunAt) < time.Duration(check.IntervalSeconds)*time.Second {
			continue
		}
		due = append(due, check)
	}
	return due, nil
}

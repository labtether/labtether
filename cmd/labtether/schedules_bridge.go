package main

import (
	"context"

	schedulespkg "github.com/labtether/labtether/internal/hubapi/schedulespkg"
	"github.com/labtether/labtether/internal/persistence"
)

// buildSchedulesDeps constructs the schedulespkg.Deps from the apiServer's fields.
func (s *apiServer) buildSchedulesDeps() *schedulespkg.Deps {
	executionStore, _ := s.scheduleStore.(persistence.ScheduleExecutionStore)
	return &schedulespkg.Deps{
		ScheduleStore:  s.scheduleStore,
		ExecutionStore: executionStore,
		AuditStore:     s.auditStore,
		AssetStore:     s.assetStore,
		GroupStore:     s.groupStore,
		AuthStore:      s.authStore,
		APIKeyStore:    s.apiKeyStore,
		JobQueue:       s.jobQueue,
	}
}

// ensureSchedulesDeps returns the schedules deps, creating and caching on first call.
func (s *apiServer) ensureSchedulesDeps() *schedulespkg.Deps {
	s.schedulesDepsOnce.Do(func() {
		s.schedulesDeps = s.buildSchedulesDeps()
	})
	return s.schedulesDeps
}

func (s *apiServer) runScheduleRunner(ctx context.Context) {
	s.ensureSchedulesDeps().RunScheduler(ctx)
}

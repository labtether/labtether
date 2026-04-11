package main

import schedulespkg "github.com/labtether/labtether/internal/hubapi/schedulespkg"

// buildSchedulesDeps constructs the schedulespkg.Deps from the apiServer's fields.
func (s *apiServer) buildSchedulesDeps() *schedulespkg.Deps {
	return &schedulespkg.Deps{
		ScheduleStore: s.scheduleStore,
		AuditStore:    s.auditStore,
	}
}

// ensureSchedulesDeps returns the schedules deps, creating and caching on first call.
func (s *apiServer) ensureSchedulesDeps() *schedulespkg.Deps {
	s.schedulesDepsOnce.Do(func() {
		s.schedulesDeps = s.buildSchedulesDeps()
	})
	return s.schedulesDeps
}

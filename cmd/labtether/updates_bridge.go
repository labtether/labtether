package main

import (
	"net/http"
	"time"

	"github.com/labtether/labtether/internal/audit"
	updatespkg "github.com/labtether/labtether/internal/hubapi/updatespkg"
)

// buildUpdatesDeps constructs the updatespkg.Deps from the apiServer's fields.
func (s *apiServer) buildUpdatesDeps() *updatespkg.Deps {
	gfd := s.ensureGroupFeaturesDeps()
	return &updatespkg.Deps{
		UpdateStore: s.updateStore,
		AssetStore:  s.assetStore,
		GroupStore:  s.groupStore,
		JobQueue:    s.jobQueue,
		LogStore:    s.logStore,

		EvaluateGuardrails:        gfd.EvaluateGuardrails,
		ResolveGroupIDsForTargets: gfd.ResolveGroupIDsForTargets,
		EnforceRateLimit: func(w http.ResponseWriter, r *http.Request, bucket string, limit int, window time.Duration) bool {
			return s.enforceRateLimit(w, r, bucket, limit, window)
		},
		AppendAuditEventBestEffort: func(event audit.Event, logMessage string) {
			s.appendAuditEventBestEffort(event, logMessage)
		},
	}
}

// ensureUpdatesDeps returns the updates deps, creating and caching on first call.
func (s *apiServer) ensureUpdatesDeps() *updatespkg.Deps {
	s.updatesDepsOnce.Do(func() {
		s.updatesDeps = s.buildUpdatesDeps()
	})
	return s.updatesDeps
}

package main

import (
	"github.com/labtether/labtether/internal/actions"
)

// executeCommandAction is a forwarding stub. The implementation has been
// extracted to internal/hubapi/worker (ExecuteCommandAction).
func (s *apiServer) executeCommandAction(job actions.Job) actions.Result {
	return s.ensureWorkerDeps().ExecuteCommandAction(job)
}

package main

import (
	"context"
	"sync/atomic"

	"github.com/labtether/labtether/internal/jobqueue"
	workerpkg "github.com/labtether/labtether/internal/hubapi/worker"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/terminal"
	"github.com/labtether/labtether/internal/updates"
)

// buildWorkerDeps constructs the worker.Deps from the apiServer's fields.
func (s *apiServer) buildWorkerDeps() *workerpkg.Deps {
	return &workerpkg.Deps{
		TerminalStore: s.terminalStore,
		ActionStore:   s.actionStore,
		UpdateStore:   s.updateStore,
		AuditStore:    s.auditStore,
		LogStore:      s.logStore,
		PresenceStore: s.presenceStore,

		AgentMgr: s.agentMgr,

		ExecuteViaAgent: func(job terminal.CommandJob) terminal.CommandResult {
			return s.executeViaAgent(job)
		},
		ExecuteActionInProcess: func(job actions.Job) actions.Result {
			return s.executeActionInProcess(job)
		},
		ExecuteUpdateScope: func(job updates.Job, target, scope string) updates.RunResultEntry {
			return s.executeUpdateScope(job, target, scope)
		},

		Broadcast: func(eventType string, data map[string]any) {
			if s.broadcaster != nil {
				s.broadcaster.Broadcast(eventType, data)
			}
		},

		MapCommandLevel:        shared.MapCommandLevel,
		IntToUint64NonNegative: shared.IntToUint64NonNegative,
	}
}

// ensureWorkerDeps returns the worker deps, creating and caching on first call.
func (s *apiServer) ensureWorkerDeps() *workerpkg.Deps {
	if s.workerDeps != nil {
		return s.workerDeps
	}
	d := s.buildWorkerDeps()
	s.workerDeps = d
	return d
}

// Forwarding methods from apiServer to worker.Deps so that existing
// cmd/labtether/ callers keep compiling without changes.

func (s *apiServer) handleTerminalCommandJob(processed *atomic.Uint64) jobqueue.HandlerFunc {
	return s.ensureWorkerDeps().HandleTerminalCommandJob(processed)
}

func (s *apiServer) handleActionRunJob(processed *atomic.Uint64) jobqueue.HandlerFunc {
	return s.ensureWorkerDeps().HandleActionRunJob(processed)
}

func (s *apiServer) handleUpdateRunJob(processed *atomic.Uint64) jobqueue.HandlerFunc {
	return s.ensureWorkerDeps().HandleUpdateRunJob(processed)
}

func (s *apiServer) recordDeadLetter(ctx context.Context, job *jobqueue.Job, jobErr error) {
	s.ensureWorkerDeps().RecordDeadLetter(ctx, job, jobErr)
}

func (s *apiServer) runPresenceCleanup(ctx context.Context) {
	s.ensureWorkerDeps().RunPresenceCleanup(ctx)
}

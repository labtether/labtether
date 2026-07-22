package main

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/labtether/labtether/internal/actions"
	opspkg "github.com/labtether/labtether/internal/hubapi/operations"
	"github.com/labtether/labtether/internal/hubapi/shared"
	workerpkg "github.com/labtether/labtether/internal/hubapi/worker"
	"github.com/labtether/labtether/internal/jobqueue"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/policy"
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
		ScheduleStore: s.scheduleStore,
		ScheduleExecutionStore: func() persistence.ScheduleExecutionStore {
			store, _ := s.scheduleStore.(persistence.ScheduleExecutionStore)
			return store
		}(),
		AssetStore: s.assetStore,
		GroupStore: s.groupStore,

		AgentMgr: s.agentMgr,

		ExecuteViaAgent: func(job terminal.CommandJob) terminal.CommandResult {
			return s.executeViaAgent(job)
		},
		PrepareTerminalCommand: func(job *terminal.CommandJob) error {
			if job == nil || job.SSHConfig != nil || opspkg.LoadCommandExecutorConfig().Mode != opspkg.ExecutorModeSSH {
				return nil
			}
			if s.terminalStore == nil {
				return fmt.Errorf("terminal session store is unavailable")
			}
			session, ok, err := s.terminalStore.GetSession(job.SessionID)
			if err != nil {
				return fmt.Errorf("load terminal session: %w", err)
			}
			if !ok {
				return fmt.Errorf("terminal session not found")
			}
			resolved, err := s.ensureTerminalDeps().ResolveSessionSSHConfig(session)
			if err != nil {
				return err
			}
			job.SSHConfig = resolved
			return nil
		},
		ExecuteTerminalCommand: func(job terminal.CommandJob) terminal.CommandResult {
			return opspkg.ExecuteCommand(job)
		},
		ExecuteActionInProcess: func(job actions.Job) actions.Result {
			return s.executeActionInProcess(job)
		},
		ExecuteUpdateScope: func(job updates.Job, target, scope string) updates.RunResultEntry {
			return s.executeUpdateScope(job, target, scope)
		},
		EvaluateAssetGuardrails: s.ensureGroupFeaturesDeps().EvaluateAssetGuardrails,
		AuthorizeScheduleTarget: func(ctx context.Context, actorID, assetID string) error {
			return s.ensureSchedulesDeps().AuthorizeExecutionTarget(ctx, actorID, assetID)
		},
		GetPolicyConfig: func() policy.EvaluatorConfig {
			return s.policyState.Current()
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
	s.workerDepsOnce.Do(func() {
		s.workerDeps = s.buildWorkerDeps()
	})
	return s.workerDeps
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

func (s *apiServer) handleScheduleRunJob(processed *atomic.Uint64) jobqueue.HandlerFunc {
	return s.ensureWorkerDeps().HandleScheduleRunJob(processed)
}

func (s *apiServer) recordDeadLetter(ctx context.Context, job *jobqueue.Job, jobErr error) {
	s.ensureWorkerDeps().RecordDeadLetter(ctx, job, jobErr)
}

func (s *apiServer) runPresenceCleanup(ctx context.Context) {
	s.ensureWorkerDeps().RunPresenceCleanup(ctx)
}

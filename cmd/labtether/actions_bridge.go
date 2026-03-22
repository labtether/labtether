package main

import (
	"net/http"
	"time"

	"github.com/labtether/labtether/internal/audit"
	actionspkg "github.com/labtether/labtether/internal/hubapi/actionspkg"
	"github.com/labtether/labtether/internal/policy"
)

// buildActionsDeps constructs the full actionspkg.Deps from the apiServer's
// fields. Both the v2 saved-action handlers and the v1 action-execution
// handlers share the same Deps value; all fields are populated here so there
// is no risk of using an under-populated cached instance.
func (s *apiServer) buildActionsDeps() *actionspkg.Deps {
	gfd := s.ensureGroupFeaturesDeps()
	return &actionspkg.Deps{
		// Saved-action fields (v2 API).
		SavedActionStore: s.savedActionStore,
		AuditStore:       s.auditStore,
		ExecOnAsset:      s.execOnAssetForActions,

		// Action-execution fields (v1 endpoints).
		ActionStore: s.actionStore,
		AssetStore:  s.assetStore,
		GroupStore:  s.groupStore,
		JobQueue:    s.jobQueue,
		LogStore:    s.logStore,

		// Injected function fields for cross-cutting concerns.
		EvaluateGuardrails:      gfd.EvaluateGuardrails,
		ResolveGroupIDForAction: gfd.ResolveGroupIDForAction,
		EnforceRateLimit: func(w http.ResponseWriter, r *http.Request, bucket string, limit int, window time.Duration) bool {
			return s.enforceRateLimit(w, r, bucket, limit, window)
		},
		GetPolicyConfig: func() policy.EvaluatorConfig {
			return s.policyState.Current()
		},
		AppendAuditEventBestEffort: func(event audit.Event, logMessage string) {
			s.appendAuditEventBestEffort(event, logMessage)
		},
	}
}

// ensureActionsDeps returns the actions deps, creating and caching on first call.
func (s *apiServer) ensureActionsDeps() *actionspkg.Deps {
	if s.actionsDeps != nil {
		return s.actionsDeps
	}
	d := s.buildActionsDeps()
	s.actionsDeps = d
	return d
}

// ensureActionRunDeps is an alias for ensureActionsDeps; it exists so that
// the action-execution stubs in actions_handlers.go read clearly.
func (s *apiServer) ensureActionRunDeps() *actionspkg.Deps {
	return s.ensureActionsDeps()
}

// execOnAssetForActions adapts v2ExecOnAsset to the actionspkg.ExecResult type.
func (s *apiServer) execOnAssetForActions(r *http.Request, assetID, command string, timeoutSec int) actionspkg.ExecResult {
	res := s.v2ExecOnAsset(r, assetID, command, timeoutSec)
	return actionspkg.ExecResult{
		AssetID:    res.AssetID,
		ExitCode:   res.ExitCode,
		Stdout:     res.Stdout,
		DurationMs: res.DurationMs,
		Error:      res.Error,
		Message:    res.Message,
	}
}

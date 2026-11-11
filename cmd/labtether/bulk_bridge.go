package main

import (
	"errors"
	"net/http"
	"time"

	bulkpkg "github.com/labtether/labtether/internal/hubapi/bulkpkg"
	respkg "github.com/labtether/labtether/internal/hubapi/resources"
	"github.com/labtether/labtether/internal/hubapi/shared"
)

// buildBulkDeps constructs the bulkpkg.Deps from the apiServer's fields.
func (s *apiServer) buildBulkDeps() *bulkpkg.Deps {
	return &bulkpkg.Deps{
		AuditStore:              s.auditStore,
		ExecOnAsset:             s.execOnAssetForBulk,
		RunServiceAction:        s.serviceActionForBulk,
		EvaluateAssetGuardrails: s.ensureGroupFeaturesDeps().EvaluateAssetGuardrails,
		EnforceRateLimit:        s.enforceRateLimit,
	}
}

// ensureBulkDeps returns the bulk deps, creating and caching on first call.
func (s *apiServer) ensureBulkDeps() *bulkpkg.Deps {
	s.bulkDepsOnce.Do(func() {
		s.bulkDeps = s.buildBulkDeps()
	})
	return s.bulkDeps
}

// execOnAssetForBulk adapts v2ExecOnAsset to the bulkpkg.ExecResult type.
func (s *apiServer) execOnAssetForBulk(r *http.Request, assetID, command string, timeoutSec int) bulkpkg.ExecResult {
	res := s.v2ExecOnAsset(r, assetID, command, timeoutSec)
	return bulkpkg.ExecResult{
		AssetID:    res.AssetID,
		Status:     res.Status,
		ExitCode:   res.ExitCode,
		Stdout:     res.Stdout,
		DurationMs: res.DurationMs,
		Error:      res.Error,
		Message:    res.Message,
	}
}

// serviceActionForBulk runs one bulk target's service action through the
// agent's service manager, the path the device Services page uses.
func (s *apiServer) serviceActionForBulk(_ *http.Request, assetID, service, action string) bulkpkg.ExecResult {
	start := time.Now()
	result, err := s.ensureResourcesDeps().RunServiceAction(assetID, service, action)
	elapsed := time.Since(start).Milliseconds()
	switch {
	case errors.Is(err, respkg.ErrServiceAgentNotConnected):
		code := "asset_offline"
		status, message := shared.OfflineDeviceAnswer(s.assetStore, assetID)
		if status == http.StatusNotFound {
			code = "asset_not_found"
		}
		return bulkpkg.ExecResult{AssetID: assetID, Error: code, Message: message}
	case errors.Is(err, respkg.ErrServiceAgentTimeout):
		return bulkpkg.ExecResult{AssetID: assetID, Error: "agent_timeout", Message: err.Error(), DurationMs: elapsed}
	case err != nil:
		return bulkpkg.ExecResult{AssetID: assetID, Error: "agent_error", Message: err.Error(), DurationMs: elapsed}
	}
	if result.Error != "" {
		return bulkpkg.ExecResult{
			AssetID:    assetID,
			Status:     "failed",
			Stdout:     result.Output,
			DurationMs: elapsed,
			Message:    respkg.ServiceActionFailureWords(result),
		}
	}
	return bulkpkg.ExecResult{AssetID: assetID, Status: "succeeded", Stdout: result.Output, DurationMs: elapsed}
}

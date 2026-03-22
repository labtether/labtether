package main

import (
	"net/http"

	bulkpkg "github.com/labtether/labtether/internal/hubapi/bulkpkg"
)

// buildBulkDeps constructs the bulkpkg.Deps from the apiServer's fields.
func (s *apiServer) buildBulkDeps() *bulkpkg.Deps {
	return &bulkpkg.Deps{
		AuditStore:  s.auditStore,
		ExecOnAsset: s.execOnAssetForBulk,
	}
}

// ensureBulkDeps returns the bulk deps, creating and caching on first call.
func (s *apiServer) ensureBulkDeps() *bulkpkg.Deps {
	if s.bulkDeps != nil {
		return s.bulkDeps
	}
	d := s.buildBulkDeps()
	s.bulkDeps = d
	return d
}

// execOnAssetForBulk adapts v2ExecOnAsset to the bulkpkg.ExecResult type.
func (s *apiServer) execOnAssetForBulk(r *http.Request, assetID, command string, timeoutSec int) bulkpkg.ExecResult {
	res := s.v2ExecOnAsset(r, assetID, command, timeoutSec)
	return bulkpkg.ExecResult{
		AssetID:    res.AssetID,
		ExitCode:   res.ExitCode,
		Stdout:     res.Stdout,
		DurationMs: res.DurationMs,
		Error:      res.Error,
		Message:    res.Message,
	}
}

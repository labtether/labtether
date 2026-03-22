package main

import (
	"net/http"
)

// Thin stubs — behaviour lives in internal/hubapi/operations.

// handleV2AssetExec handles POST /api/v2/assets/{id}/exec.
func (s *apiServer) handleV2AssetExec(w http.ResponseWriter, r *http.Request, assetID string) {
	s.ensureOperationsDeps().HandleAssetExec(w, r, assetID)
}

// handleV2ExecMulti handles POST /api/v2/exec (multi-asset fan-out).
func (s *apiServer) handleV2ExecMulti(w http.ResponseWriter, r *http.Request) {
	s.ensureOperationsDeps().HandleExecMulti(w, r)
}

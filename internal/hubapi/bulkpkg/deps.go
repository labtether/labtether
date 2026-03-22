package bulkpkg

import (
	"net/http"

	"github.com/labtether/labtether/internal/persistence"
)

// ExecResult holds the result of executing a command on a single asset.
// It mirrors actionspkg.ExecResult and v2ExecResult in cmd/labtether without
// creating a circular import.
type ExecResult struct {
	AssetID    string `json:"asset_id"`
	ExitCode   int    `json:"exit_code"`
	Stdout     string `json:"stdout"`
	DurationMs int64  `json:"duration_ms"`
	Error      string `json:"error,omitempty"`
	Message    string `json:"message,omitempty"`
}

// Deps holds the dependencies for bulk operation handlers.
type Deps struct {
	AuditStore persistence.AuditStore
	// ExecOnAsset is injected by the cmd/labtether bridge to execute a command
	// on a single asset. Using a function field avoids importing cmd/labtether
	// (which would be a circular import).
	ExecOnAsset func(r *http.Request, assetID, command string, timeoutSec int) ExecResult
}

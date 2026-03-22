package operations

import (
	"context"
	"net/http"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/terminal"
)

// AuditEvent is a type alias for audit.Event so callers don't need to import
// the audit package just to satisfy the Deps interface.
type AuditEvent = audit.Event

// ExecDeps holds the dependencies required by the exec handlers.
// It is intentionally narrow — only what exec needs — to keep the surface small.
type ExecDeps struct {
	// AgentMgr is used to check connectivity and dispatch commands.
	AgentMgr *agentmgr.AgentManager

	// AssetStore is used to look up asset metadata when the agent is offline.
	AssetStore persistence.AssetStore

	// ExecuteViaAgent dispatches a command job through the connected agent.
	ExecuteViaAgent func(job terminal.CommandJob) terminal.CommandResult

	// DecodeJSONBody decodes an HTTP request body into dst.
	DecodeJSONBody func(w http.ResponseWriter, r *http.Request, dst any) error

	// PrincipalActorID extracts the actor ID from the request context.
	PrincipalActorID func(ctx context.Context) string

	// AppendAuditEventBestEffort appends an audit event, logging on failure.
	AppendAuditEventBestEffort func(event AuditEvent, logMessage string)

	// AllowedAssetsFromContext extracts the asset allowlist from the request context.
	AllowedAssetsFromContext func(ctx context.Context) []string

	// ScopesFromContext extracts the scopes from the request context.
	ScopesFromContext func(ctx context.Context) []string

	// EnforceRateLimit returns false (and writes 429) if the rate limit has
	// been exceeded for the given bucket.
	EnforceRateLimit func(w http.ResponseWriter, r *http.Request, bucket string, limit int, window time.Duration) bool
}

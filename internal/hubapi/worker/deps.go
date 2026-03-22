package worker

import (
	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/terminal"
	"github.com/labtether/labtether/internal/updates"
)

// Deps holds all dependencies required by the worker handler package.
// Store interfaces are embedded directly; cross-cutting concerns that live
// in other cmd/labtether subsystems are injected as function fields.
type Deps struct {
	// Store interfaces.
	TerminalStore persistence.TerminalStore
	ActionStore   persistence.ActionStore
	UpdateStore   persistence.UpdateStore
	AuditStore    persistence.AuditStore
	LogStore      persistence.LogStore
	PresenceStore persistence.PresenceStore

	// Agent manager for connectivity checks and command routing.
	AgentMgr *agentmgr.AgentManager

	// ExecuteViaAgent routes a command job through a connected agent WebSocket.
	// Injected from agents_bridge to avoid circular imports.
	ExecuteViaAgent func(job terminal.CommandJob) terminal.CommandResult

	// ExecuteActionInProcess runs an action job in-process (proxmox-aware path).
	// Injected from proxmox_bridge / operations so callers don't import those packages.
	ExecuteActionInProcess func(job actions.Job) actions.Result

	// ExecuteUpdateScope runs a single (target, scope) pair for an update job.
	// Injected from update_execution_runtime in cmd/labtether.
	ExecuteUpdateScope func(job updates.Job, target, scope string) updates.RunResultEntry

	// Broadcast fires a live event to connected browser clients.
	// Injected to avoid importing cmd/labtether's EventBroadcaster.
	Broadcast func(eventType string, data map[string]any)

	// MapCommandLevel converts a command status string to a log level string.
	// Injected from http_helpers / shared package alias.
	MapCommandLevel func(status string) string

	// IntToUint64NonNegative converts a (possibly negative) int to uint64.
	// Injected from env_helpers / shared package alias.
	IntToUint64NonNegative func(value int) uint64
}

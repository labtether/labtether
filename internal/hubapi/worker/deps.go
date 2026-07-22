package worker

import (
	"context"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/hubapi/maintenanceguard"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/policy"
	"github.com/labtether/labtether/internal/terminal"
	"github.com/labtether/labtether/internal/updates"
)

// Deps holds all dependencies required by the worker handler package.
// Store interfaces are embedded directly; cross-cutting concerns that live
// in other cmd/labtether subsystems are injected as function fields.
type Deps struct {
	// Store interfaces.
	TerminalStore          persistence.TerminalStore
	ActionStore            persistence.ActionStore
	UpdateStore            persistence.UpdateStore
	AuditStore             persistence.AuditStore
	LogStore               persistence.LogStore
	PresenceStore          persistence.PresenceStore
	ScheduleStore          persistence.ScheduleStore
	ScheduleExecutionStore persistence.ScheduleExecutionStore
	AssetStore             persistence.AssetStore
	GroupStore             persistence.GroupStore

	// Agent manager for connectivity checks and command routing.
	AgentMgr *agentmgr.AgentManager

	// ExecuteViaAgent routes a command job through a connected agent WebSocket.
	// Injected from agents_bridge to avoid circular imports.
	ExecuteViaAgent func(job terminal.CommandJob) terminal.CommandResult

	// PrepareTerminalCommand resolves any ephemeral execution-only material
	// (notably decrypted SSH credentials) after the queue payload is claimed and
	// before fallback execution. The resolved values must never be persisted.
	PrepareTerminalCommand func(job *terminal.CommandJob) error

	// ExecuteTerminalCommand runs the non-agent fallback command path.
	ExecuteTerminalCommand func(job terminal.CommandJob) terminal.CommandResult

	// ExecuteActionInProcess runs an action job in-process (proxmox-aware path).
	// Injected from proxmox_bridge / operations so callers don't import those packages.
	ExecuteActionInProcess func(job actions.Job) actions.Result

	// ExecuteUpdateScope runs a single (target, scope) pair for an update job.
	// Injected from update_execution_runtime in cmd/labtether.
	ExecuteUpdateScope func(job updates.Job, target, scope string) updates.RunResultEntry

	// EvaluateAssetGuardrails re-checks maintenance at execution time so work
	// queued before a window started cannot run through block_actions.
	EvaluateAssetGuardrails maintenanceguard.EvaluateAssetFunc

	// AuthorizeScheduleTarget revalidates the creator and target scope when a
	// durable schedule occurrence reaches a worker.
	AuthorizeScheduleTarget func(ctx context.Context, actorID, assetID string) error

	// GetPolicyConfig returns the current policy rather than the policy that was
	// active when the schedule was saved.
	GetPolicyConfig func() policy.EvaluatorConfig

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

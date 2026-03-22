package agentcore

import (
	"context"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
)

// Collector emits periodic telemetry samples.
type Collector interface {
	Collect(now time.Time) (TelemetrySample, error)
}

// MetadataProvider returns static metadata attached to every heartbeat.
type MetadataProvider interface {
	StaticMetadata() map[string]string
}

// AgentInfoProvider returns endpoint-helper identity/runtime info.
type AgentInfoProvider interface {
	AgentInfo() AgentInfo
}

// CommandExecutor is the future command runner contract for endpoint-helper mode.
type CommandExecutor interface {
	Execute(ctx context.Context, command string) (string, error)
}

// HeartbeatPublisher delivers normalized telemetry heartbeats.
type HeartbeatPublisher interface {
	Publish(ctx context.Context, sample TelemetrySample) error
}

// Transport abstracts the agent-to-hub communication channel.
type Transport interface {
	Connect(ctx context.Context) error
	Send(msg agentmgr.Message) error
	Receive() (agentmgr.Message, error)
	Close()
	Connected() bool
}

// TelemetryProvider combines collection and metadata contracts.
type TelemetryProvider interface {
	Collector
	MetadataProvider
	AgentInfoProvider
}

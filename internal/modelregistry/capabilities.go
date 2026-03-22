package modelregistry

import "github.com/labtether/labtether/internal/model"

var capabilityCatalog = []model.CapabilitySpec{
	{ID: "inventory.discover", Scope: model.CapabilityScopeRead, Stability: model.CapabilityStabilityGA},
	{ID: "health.read", Scope: model.CapabilityScopeRead, Stability: model.CapabilityStabilityGA},
	{ID: "telemetry.read", Scope: model.CapabilityScopeRead, Stability: model.CapabilityStabilityGA},
	{ID: "events.read", Scope: model.CapabilityScopeRead, Stability: model.CapabilityStabilityGA},
	{ID: "events.stream", Scope: model.CapabilityScopeStream, Stability: model.CapabilityStabilityGA},
	{ID: "logs.read", Scope: model.CapabilityScopeRead, Stability: model.CapabilityStabilityGA},
	{ID: "logs.query", Scope: model.CapabilityScopeRead, Stability: model.CapabilityStabilityGA},
	{ID: "logs.stream", Scope: model.CapabilityScopeStream, Stability: model.CapabilityStabilityGA},
	{ID: "terminal.open", Scope: model.CapabilityScopeAction, Stability: model.CapabilityStabilityGA},
	{ID: "files.read", Scope: model.CapabilityScopeRead, Stability: model.CapabilityStabilityGA},
	{ID: "files.write", Scope: model.CapabilityScopeAction, Stability: model.CapabilityStabilityGA},
	{ID: "files.list", Scope: model.CapabilityScopeRead, Stability: model.CapabilityStabilityGA},
	{ID: "process.list", Scope: model.CapabilityScopeRead, Stability: model.CapabilityStabilityGA},
	{ID: "service.list", Scope: model.CapabilityScopeRead, Stability: model.CapabilityStabilityGA},
	{ID: "service.action", Scope: model.CapabilityScopeAction, Stability: model.CapabilityStabilityGA},
	{ID: "network.list", Scope: model.CapabilityScopeRead, Stability: model.CapabilityStabilityGA},
	{ID: "network.action", Scope: model.CapabilityScopeAction, Stability: model.CapabilityStabilityGA},
	{ID: "package.list", Scope: model.CapabilityScopeRead, Stability: model.CapabilityStabilityGA},
	{ID: "package.action", Scope: model.CapabilityScopeAction, Stability: model.CapabilityStabilityGA},
	{ID: "cron.list", Scope: model.CapabilityScopeRead, Stability: model.CapabilityStabilityGA},
	{ID: "users.list", Scope: model.CapabilityScopeRead, Stability: model.CapabilityStabilityGA},
	{ID: "workload.action", Scope: model.CapabilityScopeAction, Stability: model.CapabilityStabilityGA},
	{ID: "snapshot.action", Scope: model.CapabilityScopeAction, Stability: model.CapabilityStabilityGA},
	{ID: "backup.action", Scope: model.CapabilityScopeAction, Stability: model.CapabilityStabilityGA},
	{ID: "image.action", Scope: model.CapabilityScopeAction, Stability: model.CapabilityStabilityGA},
	{ID: "stack.action", Scope: model.CapabilityScopeAction, Stability: model.CapabilityStabilityGA},
	{ID: "system.action", Scope: model.CapabilityScopeAction, Stability: model.CapabilityStabilityGA},
}

func CapabilityCatalog() []model.CapabilitySpec {
	if len(capabilityCatalog) == 0 {
		return nil
	}
	out := make([]model.CapabilitySpec, len(capabilityCatalog))
	copy(out, capabilityCatalog)
	return out
}

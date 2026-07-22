package shared

import (
	"context"
	"strings"
)

type agentTokenIDContextKey struct{}
type existingAgentHeartbeatOnlyContextKey struct{}
type sharedAgentHeartbeatDisabledContextKey struct{}

// ContextWithAgentTokenID carries the already-authenticated database token ID
// to the one legacy heartbeat handler that accepts per-agent bearer auth. The
// raw bearer and its hash are intentionally never placed in request context.
func ContextWithAgentTokenID(ctx context.Context, tokenID string) context.Context {
	return context.WithValue(ctx, agentTokenIDContextKey{}, strings.TrimSpace(tokenID))
}

func AgentTokenIDFromContext(ctx context.Context) string {
	value, _ := ctx.Value(agentTokenIDContextKey{}).(string)
	return strings.TrimSpace(value)
}

// ContextWithExistingAgentHeartbeatOnly marks an authenticated owner/API-key
// heartbeat request as unable to create an agent-sourced asset. The marker is
// applied by the production auth wrapper rather than inferred from a principal
// value so direct handler fixtures can continue exercising the generic upsert
// path without weakening the HTTP route.
func ContextWithExistingAgentHeartbeatOnly(ctx context.Context) context.Context {
	return context.WithValue(ctx, existingAgentHeartbeatOnlyContextKey{}, true)
}

func ExistingAgentHeartbeatOnlyFromContext(ctx context.Context) bool {
	value, _ := ctx.Value(existingAgentHeartbeatOnlyContextKey{}).(bool)
	return value
}

// ContextWithSharedAgentHeartbeatDisabled marks operator-authenticated legacy
// heartbeat requests as unable to submit source=agent. Non-agent fixture and
// integration heartbeats retain normal operator API behavior.
func ContextWithSharedAgentHeartbeatDisabled(ctx context.Context) context.Context {
	return context.WithValue(ctx, sharedAgentHeartbeatDisabledContextKey{}, true)
}

func SharedAgentHeartbeatDisabledFromContext(ctx context.Context) bool {
	value, _ := ctx.Value(sharedAgentHeartbeatDisabledContextKey{}).(bool)
	return value
}

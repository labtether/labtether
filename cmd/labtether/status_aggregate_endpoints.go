package main

import (
	"context"

	statusaggpkg "github.com/labtether/labtether/internal/hubapi/statusagg"
)

// status_aggregate_endpoints.go — thin stub.
//
// All implementation has moved to internal/hubapi/statusagg/.
// Type aliases and package-variable shims are provided so test files and other
// callers in cmd/labtether/ continue to compile without modification.

// Type aliases for types used by tests.
type statusEndpointResult = statusaggpkg.EndpointResult
type statusEndpointTarget = statusaggpkg.EndpointTarget
type statusResolvedRoutingURL = statusaggpkg.ResolvedRoutingURL

// statusProbeEndpointFunc is a shim around the exported package variable so
// tests can temporarily swap the probe implementation.
//
// Assignment to this shim forwards to the canonical variable in statusagg.
// The shim is only used in cmd/labtether test files.
var statusProbeEndpointFunc = func(ctx context.Context, target statusEndpointTarget) statusEndpointResult {
	return statusaggpkg.ProbeEndpointFunc(ctx, target)
}

// statusEndpointTargets wraps the exported function for test callers that use
// the old package-level name.
func statusEndpointTargets(
	apiBaseURL, agentBaseURL statusResolvedRoutingURL,
	dockerHostedHub bool,
) []statusEndpointTarget {
	return statusaggpkg.BuildEndpointTargets(apiBaseURL, agentBaseURL, dockerHostedHub)
}

package main

import (
	"context"

	statusaggpkg "github.com/labtether/labtether/internal/hubapi/statusagg"
)

// status_aggregate_endpoints.go — thin stub.
//
// All implementation has moved to internal/hubapi/statusagg/.
// The package-variable shim below lets cmd/labtether tests swap the probe.

// statusProbeEndpointFunc is a shim around the exported package variable so
// tests can temporarily swap the probe implementation.
//
// Assignment to this shim forwards to the canonical variable in statusagg.
// The shim is only used in cmd/labtether test files.
var statusProbeEndpointFunc = func(ctx context.Context, target statusaggpkg.EndpointTarget) statusaggpkg.EndpointResult {
	return statusaggpkg.ProbeEndpointFunc(ctx, target)
}

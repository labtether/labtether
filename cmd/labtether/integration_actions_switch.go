package main

import (
	"errors"
	"net/http"
	"regexp"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/servicehttp"
)

// Settings › Integration actions (runtime setting policy.connector_enabled)
// turns connector actions off. Only the Actions page honoured it; the device
// pages, the connector API, the CLI and MCP still changed the systems.

const (
	integrationActionsOffMessage = "Integration actions are turned off in Settings."
	terminalSessionsOffMessage   = "Terminal sessions are turned off in Settings."
)

var (
	errIntegrationActionsOff = errors.New(integrationActionsOffMessage)
	// Test connection is a read. It is the only POST exempt from the switch, and
	// only on its own route, matched on both the escaped and the decoded path so
	// an encoded slash cannot fake it.
	connectorTestRoute = regexp.MustCompile(`^(/api/v2)?/connectors/[^/]+/test$`)
	// A Portainer container shell: a GET WebSocket that runs commands.
	portainerExecRoute = regexp.MustCompile(`^(/api/v2)?/portainer/assets/[^/]+/containers/[^/]+/exec$`)
)

func (s *apiServer) integrationActionsOn() bool {
	if s.policyState == nil {
		return true
	}
	return s.policyState.Current().ConnectorEnabled
}

// refuseIntegrationActionWhenOff answers 403 for a connector write while the
// switch is off and reports whether it did. Reads are never refused.
func (s *apiServer) refuseIntegrationActionWhenOff(w http.ResponseWriter, r *http.Request) bool {
	if !apiv2.IsMutatingMethod(r.Method) || s.integrationActionsOn() {
		return false
	}
	servicehttp.WriteError(w, http.StatusForbidden, integrationActionsOffMessage)
	return true
}

// refuseConnectorRouteWhenOff is the guard of /connectors/…: like
// refuseIntegrationActionWhenOff, but Test connection stays available.
func (s *apiServer) refuseConnectorRouteWhenOff(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodPost && connectorTestRoute.MatchString(r.URL.EscapedPath()) && connectorTestRoute.MatchString(r.URL.Path) {
		return false
	}
	return s.refuseIntegrationActionWhenOff(w, r)
}

// refusePortainerShellWhenOff applies Settings › Terminal sessions to a
// Portainer container shell, as it applies to every other terminal.
func (s *apiServer) refusePortainerShellWhenOff(w http.ResponseWriter, r *http.Request) bool {
	if s.policyState == nil || s.policyState.Current().InteractiveEnabled {
		return false
	}
	if !portainerExecRoute.MatchString(r.URL.EscapedPath()) && !portainerExecRoute.MatchString(r.URL.Path) {
		return false
	}
	servicehttp.WriteError(w, http.StatusForbidden, terminalSessionsOffMessage)
	return true
}

func (s *apiServer) guardIntegrationActions(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.refuseIntegrationActionWhenOff(w, r) {
			return
		}
		next(w, r)
	}
}

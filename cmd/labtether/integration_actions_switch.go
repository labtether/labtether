package main

import (
	"errors"
	"net/http"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/servicehttp"
)

// Settings › Integration actions (runtime setting policy.connector_enabled)
// turns connector actions off. Only the Actions page honoured it; the device
// pages, the connector API, the CLI and MCP still changed the systems.
//
// The connector routes below take no exemption, so a write is refused on its
// method alone. /connectors/{id}/… (which has Test connection) and Portainer's
// container shell decide inside their own handlers, on their own route parse.

const integrationActionsOffMessage = "Integration actions are turned off in Settings."

var errIntegrationActionsOff = errors.New(integrationActionsOffMessage)

func (s *apiServer) integrationActionsOn() bool {
	if s.policyState == nil {
		return true
	}
	return s.policyState.Current().ConnectorEnabled
}

// terminalSessionsOn is Settings › Terminal sessions, for a Portainer
// container shell as for every other terminal.
func (s *apiServer) terminalSessionsOn() bool {
	if s.policyState == nil {
		return true
	}
	return s.policyState.Current().InteractiveEnabled
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

func (s *apiServer) guardIntegrationActions(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.refuseIntegrationActionWhenOff(w, r) {
			return
		}
		next(w, r)
	}
}

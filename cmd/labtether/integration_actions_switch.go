package main

import (
	"errors"
	"net/http"
	"strings"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/servicehttp"
)

// Settings › Integration actions (runtime setting policy.connector_enabled)
// turns connector actions off. Only the Actions page honoured it; the device
// pages, the connector API, the CLI and MCP still changed the systems.

const integrationActionsOffMessage = "Integration actions are turned off in Settings."

var errIntegrationActionsOff = errors.New(integrationActionsOffMessage)

func (s *apiServer) integrationActionsOn() bool {
	if s.policyState == nil {
		return true
	}
	return s.policyState.Current().ConnectorEnabled
}

// refuseIntegrationActionWhenOff answers 403 for a connector write while the
// switch is off and reports whether it did. Reads, and Test connection, are
// never refused.
func (s *apiServer) refuseIntegrationActionWhenOff(w http.ResponseWriter, r *http.Request) bool {
	if !apiv2.IsMutatingMethod(r.Method) || s.integrationActionsOn() {
		return false
	}
	if strings.HasSuffix(strings.TrimRight(r.URL.Path, "/"), "/test") {
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

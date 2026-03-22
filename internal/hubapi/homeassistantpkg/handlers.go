package homeassistantpkg

// Home Assistant v2 endpoints — placeholder until HA connector is fully implemented.
// These check scope and return 501 if the connector is not available.

import (
	"net/http"

	"github.com/labtether/labtether/internal/apiv2"
)

// HandleV2HAEntities handles GET/POST /api/v2/homeassistant/entities.
func (d *Deps) HandleV2HAEntities(w http.ResponseWriter, r *http.Request) {
	scope := "homeassistant:read"
	if r.Method == http.MethodPost {
		scope = "homeassistant:write"
	}
	if !apiv2.ScopeCheck(apiv2.ScopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	apiv2.WriteError(w, http.StatusNotImplemented, "not_implemented",
		"Home Assistant integration is not yet fully implemented")
}

// HandleV2HAEntityActions handles GET/POST /api/v2/homeassistant/entity-actions.
func (d *Deps) HandleV2HAEntityActions(w http.ResponseWriter, r *http.Request) {
	scope := "homeassistant:read"
	if r.Method == http.MethodPost {
		scope = "homeassistant:write"
	}
	if !apiv2.ScopeCheck(apiv2.ScopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	apiv2.WriteError(w, http.StatusNotImplemented, "not_implemented",
		"Home Assistant integration is not yet fully implemented")
}

// HandleV2HAAutomations handles GET /api/v2/homeassistant/automations.
func (d *Deps) HandleV2HAAutomations(w http.ResponseWriter, r *http.Request) {
	if !apiv2.ScopeCheck(apiv2.ScopesFromContext(r.Context()), "homeassistant:read") {
		apiv2.WriteScopeForbidden(w, "homeassistant:read")
		return
	}
	apiv2.WriteError(w, http.StatusNotImplemented, "not_implemented",
		"Home Assistant integration is not yet fully implemented")
}

// HandleV2HAScenes handles GET /api/v2/homeassistant/scenes.
func (d *Deps) HandleV2HAScenes(w http.ResponseWriter, r *http.Request) {
	if !apiv2.ScopeCheck(apiv2.ScopesFromContext(r.Context()), "homeassistant:read") {
		apiv2.WriteScopeForbidden(w, "homeassistant:read")
		return
	}
	apiv2.WriteError(w, http.StatusNotImplemented, "not_implemented",
		"Home Assistant integration is not yet fully implemented")
}

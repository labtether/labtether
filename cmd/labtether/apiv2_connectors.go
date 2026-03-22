package main

import (
	"net/http"
	"strings"

	"github.com/labtether/labtether/internal/apiv2"
)

func (s *apiServer) handleV2Connectors(w http.ResponseWriter, r *http.Request) {
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "connectors:read") {
		apiv2.WriteScopeForbidden(w, "connectors:read")
		return
	}
	r.URL.Path = "/connectors"
	apiv2.WrapV1Handler(s.handleListConnectors)(w, r)
}

func (s *apiServer) handleV2ConnectorActions(w http.ResponseWriter, r *http.Request) {
	scope := "connectors:read"
	if apiv2.IsMutatingMethod(r.Method) {
		scope = "connectors:write"
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	r.URL.Path = strings.Replace(r.URL.Path, "/api/v2/", "/", 1)
	apiv2.WrapV1Handler(s.handleConnectorActions)(w, r)
}

// Proxmox v2 wrappers

func (s *apiServer) handleV2ProxmoxClusterStatus(w http.ResponseWriter, r *http.Request) {
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "connectors:read") {
		apiv2.WriteScopeForbidden(w, "connectors:read")
		return
	}
	r.URL.Path = "/proxmox/cluster/status"
	apiv2.WrapV1Handler(s.handleProxmoxClusterStatus)(w, r)
}

func (s *apiServer) handleV2ProxmoxClusterResources(w http.ResponseWriter, r *http.Request) {
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "connectors:read") {
		apiv2.WriteScopeForbidden(w, "connectors:read")
		return
	}
	r.URL.Path = "/proxmox/cluster/resources"
	apiv2.WrapV1Handler(s.handleProxmoxClusterResources)(w, r)
}

func (s *apiServer) handleV2ProxmoxAssets(w http.ResponseWriter, r *http.Request) {
	scope := "connectors:read"
	if r.Method == http.MethodPost {
		scope = "connectors:write"
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	r.URL.Path = strings.Replace(r.URL.Path, "/api/v2/", "/", 1)
	apiv2.WrapV1Handler(s.handleProxmoxAssets)(w, r)
}

func (s *apiServer) handleV2ProxmoxNodeRoutes(w http.ResponseWriter, r *http.Request) {
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "connectors:read") {
		apiv2.WriteScopeForbidden(w, "connectors:read")
		return
	}
	r.URL.Path = strings.Replace(r.URL.Path, "/api/v2/", "/", 1)
	apiv2.WrapV1Handler(s.handleProxmoxNodeRoutes)(w, r)
}

func (s *apiServer) handleV2ProxmoxCephStatus(w http.ResponseWriter, r *http.Request) {
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "connectors:read") {
		apiv2.WriteScopeForbidden(w, "connectors:read")
		return
	}
	r.URL.Path = "/proxmox/ceph/status"
	apiv2.WrapV1Handler(s.handleProxmoxCephStatus)(w, r)
}

func (s *apiServer) handleV2ProxmoxTasks(w http.ResponseWriter, r *http.Request) {
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "connectors:read") {
		apiv2.WriteScopeForbidden(w, "connectors:read")
		return
	}
	r.URL.Path = strings.Replace(r.URL.Path, "/api/v2/", "/", 1)
	apiv2.WrapV1Handler(s.handleProxmoxTaskRoutes)(w, r)
}

// TrueNAS v2 wrappers

func (s *apiServer) handleV2TrueNASAssets(w http.ResponseWriter, r *http.Request) {
	scope := "connectors:read"
	if r.Method == http.MethodPost {
		scope = "connectors:write"
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	r.URL.Path = strings.Replace(r.URL.Path, "/api/v2/", "/", 1)
	apiv2.WrapV1Handler(s.handleTrueNASAssets)(w, r)
}

// PBS v2 wrappers

func (s *apiServer) handleV2PBSAssets(w http.ResponseWriter, r *http.Request) {
	scope := "connectors:read"
	if r.Method == http.MethodPost {
		scope = "connectors:write"
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	r.URL.Path = strings.Replace(r.URL.Path, "/api/v2/", "/", 1)
	apiv2.WrapV1Handler(s.handlePBSAssets)(w, r)
}

func (s *apiServer) handleV2PBSTasks(w http.ResponseWriter, r *http.Request) {
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "connectors:read") {
		apiv2.WriteScopeForbidden(w, "connectors:read")
		return
	}
	r.URL.Path = strings.Replace(r.URL.Path, "/api/v2/", "/", 1)
	apiv2.WrapV1Handler(s.handlePBSTaskRoutes)(w, r)
}

// Portainer v2 wrappers

func (s *apiServer) handleV2PortainerAssets(w http.ResponseWriter, r *http.Request) {
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "connectors:read") {
		apiv2.WriteScopeForbidden(w, "connectors:read")
		return
	}
	r.URL.Path = strings.Replace(r.URL.Path, "/api/v2/", "/", 1)
	apiv2.WrapV1Handler(s.handlePortainerAssets)(w, r)
}

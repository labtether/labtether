package main

import (
	"github.com/labtether/labtether/internal/apiv2"
	"net/http"
	"strings"
)

// --- Discovery ---

func (s *apiServer) handleV2DiscoveryRun(w http.ResponseWriter, r *http.Request) {
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "discovery:write") {
		apiv2.WriteScopeForbidden(w, "discovery:write")
		return
	}
	r.URL.Path = "/discovery/run"
	apiv2.WrapV1Handler(s.handleDiscoveryRun)(w, r)
}

func (s *apiServer) handleV2DiscoveryProposals(w http.ResponseWriter, r *http.Request) {
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "discovery:read") {
		apiv2.WriteScopeForbidden(w, "discovery:read")
		return
	}
	r.URL.Path = "/discovery/proposals"
	apiv2.WrapV1Handler(s.handleProposals)(w, r)
}

func (s *apiServer) handleV2DiscoveryProposalActions(w http.ResponseWriter, r *http.Request) {
	scope := "discovery:read"
	if r.Method == http.MethodPost {
		scope = "discovery:write"
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	r.URL.Path = strings.Replace(r.URL.Path, "/api/v2/discovery/proposals/", "/discovery/proposals/", 1)
	apiv2.WrapV1Handler(s.handleProposalActions)(w, r)
}

// --- Topology ---

func (s *apiServer) handleV2Dependencies(w http.ResponseWriter, r *http.Request) {
	scope := "topology:read"
	if r.Method == http.MethodPost || r.Method == http.MethodDelete {
		scope = "topology:write"
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	r.URL.Path = "/dependencies"
	apiv2.WrapV1Handler(s.handleDependencies)(w, r)
}

func (s *apiServer) handleV2DependencyActions(w http.ResponseWriter, r *http.Request) {
	scope := "topology:read"
	if apiv2.IsMutatingMethod(r.Method) {
		scope = "topology:write"
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	r.URL.Path = strings.Replace(r.URL.Path, "/api/v2/dependencies/", "/dependencies/", 1)
	apiv2.WrapV1Handler(s.handleDependencyActions)(w, r)
}

func (s *apiServer) handleV2Edges(w http.ResponseWriter, r *http.Request) {
	scope := "topology:read"
	if apiv2.IsMutatingMethod(r.Method) {
		scope = "topology:write"
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	r.URL.Path = "/edges"
	apiv2.WrapV1Handler(s.handleEdges)(w, r)
}

func (s *apiServer) handleV2EdgeActions(w http.ResponseWriter, r *http.Request) {
	scope := "topology:read"
	if apiv2.IsMutatingMethod(r.Method) {
		scope = "topology:write"
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	r.URL.Path = strings.Replace(r.URL.Path, "/api/v2/edges/", "/edges/", 1)
	apiv2.WrapV1Handler(s.handleEdgeByID)(w, r)
}

func (s *apiServer) handleV2Composites(w http.ResponseWriter, r *http.Request) {
	scope := "topology:read"
	if apiv2.IsMutatingMethod(r.Method) {
		scope = "topology:write"
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	r.URL.Path = "/composites"
	apiv2.WrapV1Handler(s.handleComposites)(w, r)
}

func (s *apiServer) handleV2CompositeActions(w http.ResponseWriter, r *http.Request) {
	scope := "topology:read"
	if apiv2.IsMutatingMethod(r.Method) {
		scope = "topology:write"
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	r.URL.Path = strings.Replace(r.URL.Path, "/api/v2/composites/", "/composites/", 1)
	apiv2.WrapV1Handler(s.handleCompositeActions)(w, r)
}

// --- Failover ---

func (s *apiServer) handleV2FailoverPairs(w http.ResponseWriter, r *http.Request) {
	scope := "failover:read"
	if r.Method == http.MethodPost {
		scope = "failover:write"
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	r.URL.Path = "/group-failover-pairs"
	apiv2.WrapV1Handler(s.handleFailoverPairs)(w, r)
}

func (s *apiServer) handleV2FailoverPairActions(w http.ResponseWriter, r *http.Request) {
	scope := "failover:read"
	if apiv2.IsMutatingMethod(r.Method) {
		scope = "failover:write"
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	r.URL.Path = strings.Replace(r.URL.Path, "/api/v2/failover-pairs/", "/group-failover-pairs/", 1)
	apiv2.WrapV1Handler(s.handleFailoverPairActions)(w, r)
}

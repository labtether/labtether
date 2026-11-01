package main

import (
	"net/http"
	"strings"

	"github.com/labtether/labtether/internal/apiv2"
)

func (s *apiServer) handleV2DockerHosts(w http.ResponseWriter, r *http.Request) {
	scope := "docker:read"
	if r.Method == http.MethodPost {
		scope = "docker:write"
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	r.URL.Path = "/api/v1/docker/hosts"
	apiv2.WrapV1Handler(s.handleDockerHosts)(w, r)
}

func (s *apiServer) handleV2DockerHostActions(w http.ResponseWriter, r *http.Request) {
	scope := "docker:read"
	if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodDelete {
		scope = "docker:write"
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	// Rewrite path from /api/v2/docker/hosts/... to /api/v1/docker/hosts/...
	r.URL.Path = strings.Replace(r.URL.Path, "/api/v2/", "/api/v1/", 1)
	apiv2.WrapV1Handler(s.handleDockerHostActions)(w, r)
}

func (s *apiServer) handleV2DockerContainerActions(w http.ResponseWriter, r *http.Request) {
	scope := "docker:read"
	if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodDelete {
		scope = "docker:write"
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	r.URL.Path = strings.Replace(r.URL.Path, "/api/v2/", "/api/v1/", 1)
	s.resolveV2DockerContainerID(r)
	apiv2.WrapV1Handler(s.handleDockerContainerActions)(w, r)
}

// resolveV2DockerContainerID lets v2 callers use the raw Docker ID that
// /api/v2/docker/hosts/{id}/containers returns. It rewrites that ID to the
// container asset ID before the v1 handler checks access and looks it up.
func (s *apiServer) resolveV2DockerContainerID(r *http.Request) {
	const prefix = "/api/v1/docker/containers/"
	id, tail, hasTail := strings.Cut(strings.TrimPrefix(r.URL.Path, prefix), "/")
	if s.dockerCoordinator == nil || strings.HasPrefix(id, "docker-ct-") {
		return
	}
	assetID, ok := s.dockerCoordinator.ContainerAssetIDForDockerID(id)
	if !ok {
		return
	}
	r.URL.Path = prefix + assetID
	if hasTail {
		r.URL.Path += "/" + tail
	}
	r.URL.RawPath = ""
}

func (s *apiServer) handleV2DockerStackActions(w http.ResponseWriter, r *http.Request) {
	scope := "docker:read"
	if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodDelete {
		scope = "docker:write"
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	r.URL.Path = strings.Replace(r.URL.Path, "/api/v2/", "/api/v1/", 1)
	apiv2.WrapV1Handler(s.handleDockerStackActions)(w, r)
}

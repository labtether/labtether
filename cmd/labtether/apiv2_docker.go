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
	apiv2.WrapV1Handler(s.handleDockerContainerActions)(w, r)
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

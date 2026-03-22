package main

import (
	"net/http"
	"strings"

	"github.com/labtether/labtether/internal/apiv2"
)

func (s *apiServer) handleV2Groups(w http.ResponseWriter, r *http.Request) {
	scope := "groups:read"
	if r.Method == http.MethodPost {
		scope = "groups:write"
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	r.URL.Path = "/groups"
	apiv2.WrapV1Handler(s.handleGroups)(w, r)
}

func (s *apiServer) handleV2GroupActions(w http.ResponseWriter, r *http.Request) {
	scope := "groups:read"
	if apiv2.IsMutatingMethod(r.Method) {
		scope = "groups:write"
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	r.URL.Path = strings.Replace(r.URL.Path, "/api/v2/", "/", 1)
	apiv2.WrapV1Handler(s.handleGroupActions)(w, r)
}

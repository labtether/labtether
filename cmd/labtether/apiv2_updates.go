package main

import (
	"net/http"
	"strings"

	"github.com/labtether/labtether/internal/apiv2"
)

func (s *apiServer) handleV2UpdatePlans(w http.ResponseWriter, r *http.Request) {
	scope := "updates:read"
	if r.Method == http.MethodPost {
		scope = "updates:write"
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	r.URL.Path = "/updates/plans"
	apiv2.WrapV1Handler(s.handleUpdatePlans)(w, r)
}

func (s *apiServer) handleV2UpdatePlanActions(w http.ResponseWriter, r *http.Request) {
	scope := "updates:read"
	if apiv2.IsMutatingMethod(r.Method) {
		scope = "updates:write"
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	r.URL.Path = strings.Replace(r.URL.Path, "/api/v2/", "/", 1)
	apiv2.WrapV1Handler(s.handleUpdatePlanActions)(w, r)
}

func (s *apiServer) handleV2UpdateRuns(w http.ResponseWriter, r *http.Request) {
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "updates:read") {
		apiv2.WriteScopeForbidden(w, "updates:read")
		return
	}
	r.URL.Path = "/updates/runs"
	apiv2.WrapV1Handler(s.handleUpdateRuns)(w, r)
}

func (s *apiServer) handleV2UpdateRunActions(w http.ResponseWriter, r *http.Request) {
	scope := "updates:read"
	if apiv2.IsMutatingMethod(r.Method) {
		scope = "updates:write"
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	r.URL.Path = strings.Replace(r.URL.Path, "/api/v2/", "/", 1)
	apiv2.WrapV1Handler(s.handleUpdateRunActions)(w, r)
}

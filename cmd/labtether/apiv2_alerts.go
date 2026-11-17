package main

import (
	"net/http"
	"strings"

	"github.com/labtether/labtether/internal/apiv2"
)

func (s *apiServer) handleV2Alerts(w http.ResponseWriter, r *http.Request) {
	if !apiv2.ScopeCheck(apiv2.ScopesFromContext(r.Context()), "alerts:read") {
		apiv2.WriteScopeForbidden(w, "alerts:read")
		return
	}
	r.URL.Path = "/alerts/instances"
	apiv2.WrapV1Handler(s.ensureAlertingDeps().HandleAlertInstances)(w, r)
}

func (s *apiServer) handleV2AlertActions(w http.ResponseWriter, r *http.Request) {
	scope := "alerts:read"
	if apiv2.IsMutatingMethod(r.Method) {
		scope = "alerts:write"
	}
	if !apiv2.ScopeCheck(apiv2.ScopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/v2/alerts/rules/") {
		r.URL.Path = strings.Replace(r.URL.Path, "/api/v2/alerts/rules/", "/alerts/rules/", 1)
		apiv2.WrapV1Handler(s.ensureAlertingDeps().HandleAlertRuleActions)(w, r)
		return
	}
	r.URL.Path = strings.Replace(r.URL.Path, "/api/v2/alerts/", "/alerts/instances/", 1)
	apiv2.WrapV1Handler(s.ensureAlertingDeps().HandleAlertInstanceActions)(w, r)
}

func (s *apiServer) handleV2AlertRules(w http.ResponseWriter, r *http.Request) {
	scope := "alerts:read"
	if apiv2.IsMutatingMethod(r.Method) {
		scope = "alerts:write"
	}
	if !apiv2.ScopeCheck(apiv2.ScopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	r.URL.Path = "/alerts/rules"
	apiv2.WrapV1Handler(s.ensureAlertingDeps().HandleAlertRules)(w, r)
}

func (s *apiServer) handleV2Incidents(w http.ResponseWriter, r *http.Request) {
	scope := "alerts:read"
	if r.Method == http.MethodPost {
		scope = "alerts:write"
	}
	if !apiv2.ScopeCheck(apiv2.ScopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	r.URL.Path = "/incidents"
	apiv2.WrapV1Handler(s.ensureAlertingDeps().HandleIncidents)(w, r)
}

func (s *apiServer) handleV2IncidentActions(w http.ResponseWriter, r *http.Request) {
	scope := "alerts:read"
	if apiv2.IsMutatingMethod(r.Method) {
		scope = "alerts:write"
	}
	if !apiv2.ScopeCheck(apiv2.ScopesFromContext(r.Context()), scope) {
		apiv2.WriteScopeForbidden(w, scope)
		return
	}
	r.URL.Path = strings.Replace(r.URL.Path, "/api/v2/", "/", 1)
	apiv2.WrapV1Handler(s.ensureAlertingDeps().HandleIncidentActions)(w, r)
}

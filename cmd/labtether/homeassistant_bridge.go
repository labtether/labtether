package main

import (
	"net/http"

	homeassistantpkg "github.com/labtether/labtether/internal/hubapi/homeassistantpkg"
)

// buildHomeAssistantDeps constructs the homeassistantpkg.Deps from apiServer's fields.
func (s *apiServer) buildHomeAssistantDeps() *homeassistantpkg.Deps {
	return &homeassistantpkg.Deps{}
}

// ensureHomeAssistantDeps returns homeassistantDeps, creating and caching on first call.
func (s *apiServer) ensureHomeAssistantDeps() *homeassistantpkg.Deps {
	if s.homeassistantDeps != nil {
		return s.homeassistantDeps
	}
	d := s.buildHomeAssistantDeps()
	s.homeassistantDeps = d
	return d
}

// Forwarding methods.

func (s *apiServer) handleV2HAEntities(w http.ResponseWriter, r *http.Request) {
	s.ensureHomeAssistantDeps().HandleV2HAEntities(w, r)
}

func (s *apiServer) handleV2HAEntityActions(w http.ResponseWriter, r *http.Request) {
	s.ensureHomeAssistantDeps().HandleV2HAEntityActions(w, r)
}

func (s *apiServer) handleV2HAAutomations(w http.ResponseWriter, r *http.Request) {
	s.ensureHomeAssistantDeps().HandleV2HAAutomations(w, r)
}

func (s *apiServer) handleV2HAScenes(w http.ResponseWriter, r *http.Request) {
	s.ensureHomeAssistantDeps().HandleV2HAScenes(w, r)
}

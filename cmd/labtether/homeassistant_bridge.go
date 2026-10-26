package main

import (
	"net/http"

	"github.com/labtether/labtether/internal/apiv2"
	homeassistantpkg "github.com/labtether/labtether/internal/hubapi/homeassistantpkg"
)

// buildHomeAssistantDeps constructs the homeassistantpkg.Deps from apiServer's fields.
func (s *apiServer) buildHomeAssistantDeps() *homeassistantpkg.Deps {
	return &homeassistantpkg.Deps{
		AssetStore:              s.assetStore,
		HubCollectorStore:       s.hubCollectorStore,
		CredentialStore:         s.credentialStore,
		SecretsManager:          s.secretsManager,
		RequireAdminAuth:        s.requireAdminAuth,
		EvaluateAssetGuardrails: s.ensureGroupFeaturesDeps().EvaluateAssetGuardrails,
	}
}

// ensureHomeAssistantDeps returns homeassistantDeps, creating and caching on first call.
func (s *apiServer) ensureHomeAssistantDeps() *homeassistantpkg.Deps {
	s.homeassistantDepsOnce.Do(func() {
		s.homeassistantDeps = s.buildHomeAssistantDeps()
	})
	s.homeassistantDeps.AssetStore = s.assetStore
	s.homeassistantDeps.HubCollectorStore = s.hubCollectorStore
	s.homeassistantDeps.CredentialStore = s.credentialStore
	s.homeassistantDeps.SecretsManager = s.secretsManager
	s.homeassistantDeps.RequireAdminAuth = s.requireAdminAuth
	s.homeassistantDeps.EvaluateAssetGuardrails = s.ensureGroupFeaturesDeps().EvaluateAssetGuardrails
	return s.homeassistantDeps
}

// Forwarding methods.

func (s *apiServer) handleV2HAEntities(w http.ResponseWriter, r *http.Request) {
	apiv2.WrapV1Handler(s.ensureHomeAssistantDeps().HandleV2HAEntities)(w, r)
}

func (s *apiServer) handleV2HAEntityActions(w http.ResponseWriter, r *http.Request) {
	apiv2.WrapV1Handler(s.ensureHomeAssistantDeps().HandleV2HAEntityActions)(w, r)
}

func (s *apiServer) handleV2HAAutomations(w http.ResponseWriter, r *http.Request) {
	apiv2.WrapV1Handler(s.ensureHomeAssistantDeps().HandleV2HAAutomations)(w, r)
}

func (s *apiServer) handleV2HAScenes(w http.ResponseWriter, r *http.Request) {
	apiv2.WrapV1Handler(s.ensureHomeAssistantDeps().HandleV2HAScenes)(w, r)
}

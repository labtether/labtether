package main

import (
	"net/http"

	portainerpkg "github.com/labtether/labtether/internal/hubapi/portainer"
	"github.com/labtether/labtether/internal/hubapi/shared"
)

// buildPortainerDeps constructs the portainerpkg.Deps from the apiServer's fields.
func (s *apiServer) buildPortainerDeps() *portainerpkg.Deps {
	return &portainerpkg.Deps{
		AssetStore:        s.assetStore,
		HubCollectorStore: s.hubCollectorStore,
		CredentialStore:   s.credentialStore,
		SecretsManager:    s.secretsManager,

		RequireAdminAuth:        s.requireAdminAuth,
		EvaluateAssetGuardrails: s.ensureGroupFeaturesDeps().EvaluateAssetGuardrails,

		WrapAuth:  s.withAuth,
		WrapAdmin: s.withAdminAuth,

		CheckSameOrigin: checkSameOrigin,

		TerminalWebSocketUpgrader:         &terminalWebSocketUpgrader,
		StartBrowserWebSocketKeepalive:    shared.StartBrowserWebSocketKeepalive,
		TouchBrowserWebSocketReadDeadline: shared.TouchBrowserWebSocketReadDeadline,
	}
}

// ensurePortainerDeps returns portainerDeps, initializing once with sync.Once
// so concurrent callers can't race on the lazy-init. Store pointers are
// refreshed from apiServer on every call so tests that swap stores mid-run
// stay visible while reusing the cached Deps for state continuity.
func (s *apiServer) ensurePortainerDeps() *portainerpkg.Deps {
	s.portainerDepsOnce.Do(func() {
		if s.portainerDeps == nil {
			s.portainerDeps = s.buildPortainerDeps()
		}
	})
	d := s.portainerDeps
	d.AssetStore = s.assetStore
	d.HubCollectorStore = s.hubCollectorStore
	d.CredentialStore = s.credentialStore
	d.SecretsManager = s.secretsManager
	d.RequireAdminAuth = s.requireAdminAuth
	d.EvaluateAssetGuardrails = s.ensureGroupFeaturesDeps().EvaluateAssetGuardrails
	d.ShellsAllowed = s.terminalSessionsOn
	return d
}

// handlePortainerAssets stays in cmd: it refuses a connector write while
// Settings › Integration actions is off (refuseIntegrationActionWhenOff, cmd
// code) before the package handler runs. The v1 route and its v2 wrapper both
// call it, so the check is in one place. Other callers call
// ensurePortainerDeps() themselves, when the request or call is made.
func (s *apiServer) handlePortainerAssets(w http.ResponseWriter, r *http.Request) {
	if s.refuseIntegrationActionWhenOff(w, r) {
		return
	}
	s.ensurePortainerDeps().HandlePortainerAssets(w, r)
}

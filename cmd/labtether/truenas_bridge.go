package main

import (
	"net/http"

	"github.com/labtether/labtether/internal/hubapi/shared"
	truenaspkg "github.com/labtether/labtether/internal/hubapi/truenas"
)

// buildTruenasDeps constructs the truenaspkg.Deps from the apiServer's fields.
func (s *apiServer) buildTruenasDeps() *truenaspkg.Deps {
	return &truenaspkg.Deps{
		AssetStore:        s.assetStore,
		HubCollectorStore: s.hubCollectorStore,
		CredentialStore:   s.credentialStore,
		SecretsManager:    s.secretsManager,
		LogStore:          s.logStore,

		TruenasSubs: make(map[string]truenaspkg.TruenasSubscriptionHandle, 8),

		RequireAdminAuth:            s.requireAdminAuth,
		EvaluateConnectorGuardrails: s.connectorMaintenanceEvaluator(),

		WrapAuth:  s.withAuth,
		WrapAdmin: s.withAdminAuth,

		CheckSameOrigin: checkSameOrigin,

		AppendConnectorLogEvent:       s.appendConnectorLogEvent,
		AppendConnectorLogEventWithID: s.appendConnectorLogEventWithID,

		StartBrowserWebSocketKeepalive:    shared.StartBrowserWebSocketKeepalive,
		TouchBrowserWebSocketReadDeadline: shared.TouchBrowserWebSocketReadDeadline,
	}
}

// ensureTruenasDeps returns truenasDeps, initializing once with sync.Once so
// concurrent callers can't race on the lazy-init. Store pointers are refreshed
// from apiServer on every call so tests that swap stores mid-run stay visible
// while reusing the cached Deps's mutable state for continuity.
func (s *apiServer) ensureTruenasDeps() *truenaspkg.Deps {
	s.truenasDepsOnce.Do(func() {
		if s.truenasDeps == nil {
			s.truenasDeps = s.buildTruenasDeps()
		}
	})
	d := s.truenasDeps
	d.AssetStore = s.assetStore
	d.HubCollectorStore = s.hubCollectorStore
	d.CredentialStore = s.credentialStore
	d.SecretsManager = s.secretsManager
	d.LogStore = s.logStore
	d.RequireAdminAuth = s.requireAdminAuth
	d.EvaluateConnectorGuardrails = s.connectorMaintenanceEvaluator()
	return d
}

// handleTrueNASAssets stays in cmd: it refuses a connector write while
// Settings › Integration actions is off (refuseIntegrationActionWhenOff, cmd
// code) before the package handler runs. The v1 route and its v2 wrapper both
// call it, so the check is in one place. Other callers call
// ensureTruenasDeps() themselves, when the request or call is made.
func (s *apiServer) handleTrueNASAssets(w http.ResponseWriter, r *http.Request) {
	if s.refuseIntegrationActionWhenOff(w, r) {
		return
	}
	s.ensureTruenasDeps().HandleTrueNASAssets(w, r)
}

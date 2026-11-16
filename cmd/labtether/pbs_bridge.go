package main

import (
	"net/http"

	pbspkg "github.com/labtether/labtether/internal/hubapi/pbs"
)

// buildPBSDeps constructs the pbspkg.Deps from the apiServer's fields.
func (s *apiServer) buildPBSDeps() *pbspkg.Deps {
	return &pbspkg.Deps{
		AssetStore:        s.assetStore,
		HubCollectorStore: s.hubCollectorStore,
		CredentialStore:   s.credentialStore,
		SecretsManager:    s.secretsManager,

		RequireAdminAuth:            s.requireAdminAuth,
		EvaluateConnectorGuardrails: s.connectorMaintenanceEvaluator(),

		WrapAuth:  s.withAuth,
		WrapAdmin: s.withAdminAuth,
	}
}

// ensurePBSDeps returns pbsDeps, initializing once with sync.Once so concurrent
// callers can't race on the lazy-init. Store pointers are refreshed from
// apiServer on every call so tests that swap stores mid-run stay visible while
// reusing the cached Deps for state continuity.
func (s *apiServer) ensurePBSDeps() *pbspkg.Deps {
	s.pbsDepsOnce.Do(func() {
		if s.pbsDeps == nil {
			s.pbsDeps = s.buildPBSDeps()
		}
	})
	d := s.pbsDeps
	d.AssetStore = s.assetStore
	d.HubCollectorStore = s.hubCollectorStore
	d.CredentialStore = s.credentialStore
	d.SecretsManager = s.secretsManager
	d.RequireAdminAuth = s.requireAdminAuth
	return d
}

// The two handlers below stay in cmd: each refuses a connector write while
// Settings › Integration actions is off (refuseIntegrationActionWhenOff, cmd
// code) before the package handler runs. The v1 route and its v2 wrapper both
// call them, so the check is in one place. Other callers call ensurePBSDeps()
// themselves, when the request or call is made.

func (s *apiServer) handlePBSAssets(w http.ResponseWriter, r *http.Request) {
	if s.refuseIntegrationActionWhenOff(w, r) {
		return
	}
	s.ensurePBSDeps().HandlePBSAssets(w, r)
}

func (s *apiServer) handlePBSTaskRoutes(w http.ResponseWriter, r *http.Request) {
	if s.refuseIntegrationActionWhenOff(w, r) {
		return
	}
	s.ensurePBSDeps().HandlePBSTaskRoutes(w, r)
}

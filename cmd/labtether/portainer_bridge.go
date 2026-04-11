package main

import (
	"net/http"

	"github.com/labtether/labtether/internal/assets"
	portainerpkg "github.com/labtether/labtether/internal/hubapi/portainer"
)

// buildPortainerDeps constructs the portainerpkg.Deps from the apiServer's fields.
func (s *apiServer) buildPortainerDeps() *portainerpkg.Deps {
	return &portainerpkg.Deps{
		AssetStore:        s.assetStore,
		HubCollectorStore: s.hubCollectorStore,
		CredentialStore:   s.credentialStore,
		SecretsManager:    s.secretsManager,

		RequireAdminAuth: s.requireAdminAuth,

		WrapAuth:  s.withAuth,
		WrapAdmin: s.withAdminAuth,

		CheckSameOrigin: checkSameOrigin,

		TerminalWebSocketUpgrader:         &terminalWebSocketUpgrader,
		StartBrowserWebSocketKeepalive:    startBrowserWebSocketKeepalive,
		TouchBrowserWebSocketReadDeadline: touchBrowserWebSocketReadDeadline,
	}
}

// Type aliases for portainer types used in cmd/labtether/.
type portainerRuntime = portainerpkg.PortainerRuntime
type cachedPortainerRuntime = portainerpkg.CachedPortainerRuntime
type portainerCapabilities = portainerpkg.PortainerCapabilities

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
	return d
}

// Forwarding methods.

func (s *apiServer) handlePortainerAssets(w http.ResponseWriter, r *http.Request) {
	s.ensurePortainerDeps().HandlePortainerAssets(w, r)
}

func (s *apiServer) handlePortainerCapabilities(w http.ResponseWriter, asset assets.Asset) {
	s.ensurePortainerDeps().HandlePortainerCapabilities(w, asset)
}

func (s *apiServer) handlePortainerContainerExec(w http.ResponseWriter, r *http.Request, assetID, containerID string) {
	s.ensurePortainerDeps().HandlePortainerContainerExec(w, r, assetID, containerID)
}

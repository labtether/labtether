package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/labtether/labtether/internal/apiv2"
	adminpkg "github.com/labtether/labtether/internal/hubapi/admin"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/servicehttp"
)

// buildAdminDeps constructs the admin.Deps from the apiServer's fields.
func (s *apiServer) buildAdminDeps() *adminpkg.Deps {
	return &adminpkg.Deps{
		RuntimeStore:      s.runtimeStore,
		AuditStore:        s.auditStore,
		AdminResetStore:   s.adminResetStore,
		CredentialStore:   s.credentialStore,
		AssetStore:        s.assetStore,
		DB:                s.db,
		SecretsManager:    s.secretsManager,
		InstallStateStore: s.installStateStore,
		TLSState:          &s.tlsState,
		PolicyState:       s.policyState,

		WebServiceCoordinator: s.webServiceCoordinator,
		DockerCoordinator:     s.dockerCoordinator,

		HubIdentity: s.currentHubSSHIdentity(),
		CurrentHubIdentity: func() *shared.HubSSHIdentity {
			return s.currentHubSSHIdentity()
		},
		DataDir: s.dataDir,

		DecodeJSONBody: func(w http.ResponseWriter, r *http.Request, dst any) error {
			return shared.DecodeJSONBody(w, r, dst)
		},
		EnforceRateLimit: s.enforceRateLimit,
		AppendAuditEventBestEffort: func(event adminpkg.AuditEventAlias, logMessage string) {
			s.appendAuditEventBestEffort(event, logMessage)
		},
		ApplySecurityRuntimeOverrides: applySecurityRuntimeOverrides,
		ApplyPrometheusRemoteWriteSettings: func() error {
			if s.prometheusRemoteWriteRuntime == nil {
				if s.db != nil {
					return fmt.Errorf("prometheus remote write runtime is unavailable")
				}
				return nil
			}
			return s.prometheusRemoteWriteRuntime.Apply()
		},
		AgentDefaultsChanged: func(changedKeys, resetKeys []string) {
			s.ensureAgentsDeps().PushAgentDefaultsToConnected(changedKeys, resetKeys)
		},
		InvalidateWebServiceURLGroupingConfigCache: func() { s.ensureCollectorsDeps().InvalidateWebServiceURLGroupingConfigCache() },
		InvalidateStatusCaches:                     s.invalidateStatusCaches,
		PrincipalActorID: func(ctx context.Context) string {
			return apiv2.PrincipalActorID(ctx)
		},
		EnsureHubIdentity: func(d *adminpkg.Deps) (*shared.HubSSHIdentity, error) {
			return s.ensureCurrentHubSSHIdentity()
		},
		LoadHubPrivateKeyPEM: func(identity *shared.HubSSHIdentity) (string, error) {
			return s.loadHubPrivateKeyPEM(identity)
		},
		HubSSHKeyEnabled: s.hubSSHKeyEnabled,
		UserRoleFromContext: func(ctx context.Context) string {
			return apiv2.UserRoleFromContext(ctx)
		},
		// Wire cmd/labtether package-level vars via closures so that test
		// overrides of tailscaleLookPath, tailscaleFallbackPaths, and
		// tailscaleRunner are respected at call time, not just at Deps-build time.
		TailscaleLookPath: func(file string) (string, error) {
			return tailscaleLookPath(file)
		},
		TailscaleFallbackPaths: func() []string {
			return tailscaleFallbackPaths()
		},
		TailscaleRunnerOverride: func(timeout time.Duration, path string, args ...string) ([]byte, error) {
			return tailscaleRunner(timeout, path, args...)
		},
	}
}

// ensureAdminDeps returns adminDeps, lazily building and caching on first call.
func (s *apiServer) ensureAdminDeps() *adminpkg.Deps {
	s.adminDepsOnce.Do(func() {
		if s.adminDeps == nil {
			s.adminDeps = s.buildAdminDeps()
		}
	})
	return s.adminDeps
}

// handlePushHubKey stays in cmd: it makes sure the Hub SSH identity exists
// and holds cmd's Hub identity lock while the package pushes the key.
func (s *apiServer) handlePushHubKey(w http.ResponseWriter, r *http.Request, assetID string) {
	if _, err := s.ensureCurrentHubSSHIdentity(); err != nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "hub SSH identity is unavailable")
		return
	}
	s.hubIdentityOperationMu.Lock()
	defer s.hubIdentityOperationMu.Unlock()
	s.ensureAdminDeps().HandlePushHubKey(w, r, assetID)
}

// --- Tailscale test seams, read by tls_tailscale.go and buildAdminDeps ---

// tailscaleLookPath and tailscaleFallbackPaths are package-level vars that
// tests in cmd/labtether override to control binary discovery. They are wired
// into admin.Deps via buildAdminDeps so that the admin package's handlers
// respect test overrides.
var tailscaleLookPath = adminpkg.TailscaleLookPath
var tailscaleFallbackPaths = adminpkg.TailscaleFallbackPaths

// tailscaleRunner is a package-level var that delegates through to the admin
// package's TailscaleRunner indirection. tls_tailscale.go calls this directly.
// This wrapper ensures that test overrides of adminpkg.TailscaleRunner are
// respected even when called from this package.
var tailscaleRunner = func(timeout time.Duration, path string, args ...string) ([]byte, error) {
	return adminpkg.TailscaleRunner(timeout, path, args...)
}

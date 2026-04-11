package main

import (
	"context"
	"net/http"
	"time"

	adminpkg "github.com/labtether/labtether/internal/hubapi/admin"
	opspkg "github.com/labtether/labtether/internal/hubapi/operations"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/runtimesettings"
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

		HubIdentity: s.hubIdentity,
		DataDir:     s.dataDir,

		DecodeJSONBody: func(w http.ResponseWriter, r *http.Request, dst any) error {
			return decodeJSONBody(w, r, dst)
		},
		EnforceRateLimit: s.enforceRateLimit,
		AppendAuditEventBestEffort: func(event adminpkg.AuditEventAlias, logMessage string) {
			s.appendAuditEventBestEffort(event, logMessage)
		},
		ApplySecurityRuntimeOverrides:              applySecurityRuntimeOverrides,
		InvalidateWebServiceURLGroupingConfigCache: s.invalidateWebServiceURLGroupingConfigCache,
		InvalidateStatusCaches:                     s.invalidateStatusCaches,
		PrincipalActorID: func(ctx context.Context) string {
			return principalActorID(ctx)
		},
		EnsureHubIdentity: func(d *adminpkg.Deps) (*shared.HubSSHIdentity, error) {
			identity, err := ensureHubSSHIdentity(s)
			if err != nil {
				return nil, err
			}
			// Keep apiServer in sync.
			s.hubIdentity = identity
			return identity, nil
		},
		LoadHubPrivateKeyPEM: func(identity *shared.HubSSHIdentity) (string, error) {
			return s.loadHubPrivateKeyPEM(identity)
		},
		UserRoleFromContext: func(ctx context.Context) string {
			return userRoleFromContext(ctx)
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

// --- HTTP handler forwarding methods ---

func (s *apiServer) handleRuntimeSettings(w http.ResponseWriter, r *http.Request) {
	s.ensureAdminDeps().HandleRuntimeSettings(w, r)
}

func (s *apiServer) handleRuntimeSettingsReset(w http.ResponseWriter, r *http.Request) {
	s.ensureAdminDeps().HandleRuntimeSettingsReset(w, r)
}

func (s *apiServer) handlePrometheusTestConnection(w http.ResponseWriter, r *http.Request) {
	s.ensureAdminDeps().HandlePrometheusTestConnection(w, r)
}

func (s *apiServer) handleManagedDatabaseSettings(w http.ResponseWriter, r *http.Request) {
	s.ensureAdminDeps().HandleManagedDatabaseSettings(w, r)
}

func (s *apiServer) handleManagedDatabasePasswordReveal(w http.ResponseWriter, r *http.Request) {
	s.ensureAdminDeps().HandleManagedDatabasePasswordReveal(w, r)
}

func (s *apiServer) handleCACert(w http.ResponseWriter, r *http.Request) {
	s.ensureAdminDeps().HandleCACert(w, r)
}

func (s *apiServer) handleTLSInfo(w http.ResponseWriter, r *http.Request) {
	s.ensureAdminDeps().HandleTLSInfo(w, r)
}

func (s *apiServer) handleTLSSettings(w http.ResponseWriter, r *http.Request) {
	s.ensureAdminDeps().HandleTLSSettings(w, r)
}

func (s *apiServer) handleAdminReset(w http.ResponseWriter, r *http.Request) {
	s.ensureAdminDeps().HandleAdminReset(w, r)
}

func (s *apiServer) handleListProtocolConfigs(w http.ResponseWriter, r *http.Request, assetID string) {
	s.ensureAdminDeps().HandleListProtocolConfigs(w, r, assetID)
}

func (s *apiServer) handleCreateProtocolConfig(w http.ResponseWriter, r *http.Request, assetID string) {
	s.ensureAdminDeps().HandleCreateProtocolConfig(w, r, assetID)
}

func (s *apiServer) handleUpdateProtocolConfig(w http.ResponseWriter, r *http.Request, assetID, protocol string) {
	s.ensureAdminDeps().HandleUpdateProtocolConfig(w, r, assetID, protocol)
}

func (s *apiServer) handleDeleteProtocolConfig(w http.ResponseWriter, r *http.Request, assetID, protocol string) {
	s.ensureAdminDeps().HandleDeleteProtocolConfig(w, r, assetID, protocol)
}

func (s *apiServer) handleTestProtocolConnection(w http.ResponseWriter, r *http.Request, assetID, protocol string) {
	s.ensureAdminDeps().HandleTestProtocolConnection(w, r, assetID, protocol)
}

func (s *apiServer) handlePushHubKey(w http.ResponseWriter, r *http.Request, assetID string) {
	s.ensureAdminDeps().HandlePushHubKey(w, r, assetID)
}

// --- Prometheus settings helpers and type aliases ---

// prometheusSettingsEnabled delegates to the admin package's exported helper.
func prometheusSettingsEnabled(store interface {
	ListRuntimeSettingOverrides() (map[string]string, error)
}, key string) bool {
	return adminpkg.PrometheusSettingsEnabled(store, key)
}

// Package-level type aliases so that apiv2_advanced.go (which references these
// types by their old lowercase names) keeps compiling without modification.
type prometheusTestConnectionRequest = adminpkg.PrometheusTestConnectionRequest
type prometheusTestConnectionResponse = adminpkg.PrometheusTestConnectionResponse

// Package-level constant aliases for the Prometheus test connection rate limit
// and timeout, used by apiv2_advanced.go.
const (
	prometheusTestConnectionRateLimitKey    = adminpkg.PrometheusTestConnectionRateLimitKey
	prometheusTestConnectionRateLimitCount  = adminpkg.PrometheusTestConnectionRateLimitCount
	prometheusTestConnectionRateLimitWindow = adminpkg.PrometheusTestConnectionRateLimitWindow
	prometheusTestConnectionTimeout         = adminpkg.PrometheusTestConnectionTimeout
	errPrometheusURLRequired                = adminpkg.ErrPrometheusURLRequired
)

// testPrometheusRemoteWriteConnection delegates to the admin package.
func testPrometheusRemoteWriteConnection(ctx context.Context, url, username, password string) prometheusTestConnectionResponse {
	return adminpkg.TestPrometheusRemoteWriteConnection(ctx, url, username, password)
}

// --- TLS info aliases forwarded from admin.Deps ---

// currentTLSCertType delegates to the admin Deps for callers that reference
// it as a method on apiServer (e.g. apiv2_advanced.go).
func (s *apiServer) currentTLSCertType() string {
	return s.ensureAdminDeps().CurrentTLSCertType()
}

// activeTLSCertificateMetadata delegates to the admin Deps.
func (s *apiServer) activeTLSCertificateMetadata() (opspkg.TLSCertificateMetadata, error) {
	return s.ensureAdminDeps().ActiveTLSCertificateMetadata()
}

// buildTLSSettingsResponse delegates to the admin Deps.
func (s *apiServer) buildTLSSettingsResponse(restartRequired bool) (tlsSettingsResponse, error) {
	resp, err := s.ensureAdminDeps().BuildTLSSettingsResponse(restartRequired)
	if err != nil {
		return tlsSettingsResponse{}, err
	}
	return tlsSettingsResponse(resp), nil
}

// Type aliases that cmd/labtether callers (including tests) use directly.
type tlsSettingsResponse = adminpkg.TLSSettingsResponse
type managedDatabaseSettingsPayload = adminpkg.ManagedDatabaseSettingsPayload
type managedDatabaseRevealPayload = adminpkg.ManagedDatabaseRevealPayload

// Constant aliases for audit event types and route paths used in cmd/labtether tests.
const (
	managedDatabaseRevealAuditType = adminpkg.ManagedDatabaseRevealAuditType
	tlsSettingsRoute               = adminpkg.TLSSettingsRoute
)

// writeRuntimeSettingsPayload is the package-level alias used by the original
// settings_runtime_helpers.go callers (now removed) — kept as a bridge so any
// remaining call sites in cmd/labtether compile without changes.
func (s *apiServer) writeRuntimeSettingsPayload(w http.ResponseWriter) {
	s.ensureAdminDeps().WriteRuntimeSettingsPayload(w)
}

// --- Tailscale package-level function aliases for tls_tailscale.go ---

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

// resolveTailscaleBinaryPath delegates to the package-level vars so that
// tls_tailscale.go and test code honour any overrides set on tailscaleLookPath
// and tailscaleFallbackPaths.
func resolveTailscaleBinaryPath() (string, error) {
	return adminpkg.ResolveTailscaleBinaryPathWith(tailscaleLookPath, tailscaleFallbackPaths)
}

// parseTailscaleStatusSnapshot delegates to the admin package's exported helper.
// tls_tailscale.go uses the returned snapshot's LoggedIn and DNSName fields.
func parseTailscaleStatusSnapshot(raw []byte) adminpkg.TailscaleStatusSnapshot {
	return adminpkg.ParseTailscaleStatusSnapshot(raw)
}

// --- Tailscale serve status forwarding ---

// handleTailscaleServeStatus delegates to the admin package's Tailscale handler.
func (s *apiServer) handleTailscaleServeStatus(w http.ResponseWriter, r *http.Request) {
	s.ensureAdminDeps().HandleTailscaleServeStatus(w, r)
}

// inspectTailscaleServeStatus delegates to the admin package's inspection method.
// Called by hub_connection_resolver.go to determine healthy Tailscale HTTPS candidates.
func (s *apiServer) inspectTailscaleServeStatus() adminpkg.TailscaleServeStatusResponse {
	return s.ensureAdminDeps().InspectTailscaleServeStatus()
}

// Type aliases for tailscale types used in apiv2_advanced_test.go and
// hub_connection_resolver.go.
type tailscaleServeStatusResponse = adminpkg.TailscaleServeStatusResponse

// Const aliases for tailscale constants that remain referenced in cmd/labtether.
const (
	tailscaleServeStatusRoute = adminpkg.TailscaleServeStatusRoute
	envTailscaleManaged       = adminpkg.EnvTailscaleManaged
)

// resolveRuntimeSettingValueForDefinition delegates to the admin package helper.
// Kept here so any remaining cmd/labtether callers keep compiling.
func resolveRuntimeSettingValueForDefinition(definition runtimesettings.Definition, overrides map[string]string) (string, runtimesettings.Source) {
	return adminpkg.ResolveRuntimeSettingValueForDefinition(definition, overrides)
}

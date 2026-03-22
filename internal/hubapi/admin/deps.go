// Package admin provides HTTP handler implementations for admin, settings,
// and TLS management endpoints. It is wired into cmd/labtether via
// admin_bridge.go.
package admin

import (
	"context"
	"net/http"
	"time"

	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/connectors/docker"
	"github.com/labtether/labtether/internal/connectors/webservice"
	opspkg "github.com/labtether/labtether/internal/hubapi/operations"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/installstate"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/protocols"
	"github.com/labtether/labtether/internal/secrets"
)

// ProtocolConfigDB is the narrow interface used by protocol-config handlers.
// It matches the subset of *persistence.PostgresStore methods used here,
// allowing the handlers to be tested without a real database.
type ProtocolConfigDB interface {
	ListProtocolConfigs(ctx context.Context, assetID string) ([]*protocols.ProtocolConfig, error)
	GetProtocolConfig(ctx context.Context, assetID, protocol string) (*protocols.ProtocolConfig, error)
	SaveProtocolConfig(ctx context.Context, pc *protocols.ProtocolConfig) error
	DeleteProtocolConfig(ctx context.Context, assetID, protocol string) error
	UpdateProtocolTestResult(ctx context.Context, assetID, protocol, status, testError string) error
	UpdateProtocolConfigCredential(ctx context.Context, assetID, protocol, credentialProfileID string) error
}

// Deps holds all external dependencies for the admin, settings, and TLS
// handler package. All mutation of shared state (TLSState, caches) goes
// through the function fields and pointer fields below.
type Deps struct {
	// Store interfaces.
	RuntimeStore      persistence.RuntimeSettingsStore
	AuditStore        persistence.AuditStore
	AdminResetStore   persistence.AdminResetStore
	CredentialStore   persistence.CredentialStore
	AssetStore        persistence.AssetStore

	// DB is a direct reference to the PostgresStore for protocol config
	// operations that bypass the store interface layer.
	DB ProtocolConfigDB

	// Secrets manager for TLS key encryption and credential decryption.
	SecretsManager *secrets.Manager

	// InstallStateStore loads hub installation secrets (postgres password, etc).
	InstallStateStore *installstate.Store

	// TLSState is a pointer to the live TLS runtime state on apiServer.
	// Handlers mutate it in-place for live cert switching.
	TLSState *opspkg.TLSState

	// PolicyState is the live policy runtime state; handlers call
	// ApplyOverrides after persisting runtime setting changes.
	PolicyState PolicyStateApplier

	// Coordinators for cache flushing on admin reset.
	WebServiceCoordinator *webservice.Coordinator
	DockerCoordinator     *docker.Coordinator

	// HubIdentity is the cached hub SSH identity used for hub key push.
	// The pointer may be nil on first use; EnsureHubIdentity provides lazy init.
	HubIdentity *HubSSHIdentity

	// DataDir is the hub's data directory (used for TLS file materialization).
	DataDir string

	// Function dependencies — injected so the package remains testable
	// without pulling in the full apiServer dependency graph.

	// DecodeJSONBody decodes an HTTP request body into dst.
	DecodeJSONBody func(w http.ResponseWriter, r *http.Request, dst any) error

	// EnforceRateLimit returns false (and writes 429) when the rate limit for
	// the given bucket has been exceeded.
	EnforceRateLimit func(w http.ResponseWriter, r *http.Request, bucket string, limit int, window time.Duration) bool

	// AppendAuditEventBestEffort appends an audit event, logging on failure.
	AppendAuditEventBestEffort func(event audit.Event, logMessage string)

	// ApplySecurityRuntimeOverrides propagates security-relevant runtime
	// overrides to the global securityruntime package state.
	ApplySecurityRuntimeOverrides func(overrides map[string]string)

	// InvalidateWebServiceURLGroupingConfigCache drops the cached URL-grouping
	// config so the next request reloads from the runtime settings store.
	InvalidateWebServiceURLGroupingConfigCache func()

	// InvalidateStatusCaches drops all in-memory status-aggregate caches.
	InvalidateStatusCaches func()

	// PrincipalActorID extracts the actor ID from the request context for
	// audit attribution.
	PrincipalActorID func(ctx context.Context) string

	// EnsureHubIdentity generates or loads the hub SSH keypair. It may update
	// d.HubIdentity in place and should be called when HubIdentity is nil.
	EnsureHubIdentity func(d *Deps) (*HubSSHIdentity, error)

	// LoadHubPrivateKeyPEM decrypts and returns the hub SSH private key PEM
	// for the given identity.
	LoadHubPrivateKeyPEM func(identity *HubSSHIdentity) (string, error)

	// UserRoleFromContext extracts the user role string from the request
	// context. Used by the Tailscale serve mutation handler for admin-role
	// enforcement. If nil, the handler treats every caller as non-admin.
	UserRoleFromContext func(ctx context.Context) string

	// TailscaleLookPath overrides the LookPath implementation used for
	// locating the tailscale binary. If nil the package-level var is used.
	TailscaleLookPath func(file string) (string, error)

	// TailscaleFallbackPaths overrides the fallback path resolver used for
	// locating the tailscale binary on non-PATH systems. If nil the
	// package-level var is used.
	TailscaleFallbackPaths func() []string

	// TailscaleRunnerOverride overrides the TailscaleRunner used to invoke
	// tailscale sub-commands. If nil the package-level TailscaleRunner var is
	// used. Tests in cmd/labtether inject this to control command output.
	TailscaleRunnerOverride func(timeout time.Duration, path string, args ...string) ([]byte, error)
}

// AuditEventAlias is a type alias for audit.Event so that bridge callers do not
// need to import the audit package just to satisfy the Deps callback signature.
type AuditEventAlias = audit.Event

// PolicyStateApplier is the subset of opspkg.PolicyRuntimeState used here.
type PolicyStateApplier interface {
	ApplyOverrides(overrides map[string]string)
}

// HubSSHIdentity is a type alias for shared.HubSSHIdentity so that callers in
// cmd/labtether and the bridge file can use the shared type without an
// additional import. The bridge provides a further alias so existing code
// in hub_identity.go continues to compile.
type HubSSHIdentity = shared.HubSSHIdentity

// principalActorID is a package-local helper that delegates to the injected
// function in Deps, matching the calling convention used throughout this file.
func (d *Deps) principalActorID(ctx context.Context) string {
	if d.PrincipalActorID != nil {
		return d.PrincipalActorID(ctx)
	}
	return ""
}

// appendAuditEventBestEffort is a package-local helper delegating to the
// injected function.
func (d *Deps) appendAuditEventBestEffort(event audit.Event, logMessage string) {
	if d.AppendAuditEventBestEffort != nil {
		d.AppendAuditEventBestEffort(event, logMessage)
	}
}

// enforceRateLimit delegates to the injected rate-limit function.
func (d *Deps) enforceRateLimit(w http.ResponseWriter, r *http.Request, bucket string, limit int, window time.Duration) bool {
	if d.EnforceRateLimit != nil {
		return d.EnforceRateLimit(w, r, bucket, limit, window)
	}
	return true
}

// decodeJSONBody delegates to the injected body-decode function.
func (d *Deps) decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any) error {
	if d.DecodeJSONBody != nil {
		return d.DecodeJSONBody(w, r, dst)
	}
	return nil
}

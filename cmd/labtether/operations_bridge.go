package main

import (
	"context"
	"net/http"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/connectorsdk"
	opspkg "github.com/labtether/labtether/internal/hubapi/operations"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/policy"
	"github.com/labtether/labtether/internal/retention"
	"github.com/labtether/labtether/internal/secrets"
	"github.com/labtether/labtether/internal/terminal"
	"github.com/labtether/labtether/internal/updates"
)

// Type aliases for types used throughout cmd/labtether/.
type workerRuntimeState = opspkg.WorkerRuntimeState
type retentionState = opspkg.RetentionState
type commandExecutorConfig = opspkg.CommandExecutorConfig
type sshTarget = opspkg.SSHTarget

// Function aliases — these keep existing cmd/labtether callers compiling.
func newWorkerRuntimeState(maxDeliveries uint64, retentionInterval time.Duration) *workerRuntimeState {
	return opspkg.NewWorkerRuntimeState(maxDeliveries, retentionInterval)
}

func refreshWorkerRuntimeSettingsDirect(ctx context.Context, store persistence.RuntimeSettingsStore, state *workerRuntimeState, onApplied func(*workerRuntimeState)) {
	opspkg.RefreshWorkerRuntimeSettingsDirect(ctx, store, state, onApplied)
}

func runRetentionLoop(ctx context.Context, retentionStore persistence.RetentionStore, runtimeState *workerRuntimeState, tracker *retentionState) {
	opspkg.RunRetentionLoop(ctx, retentionStore, runtimeState, tracker)
}

func workerFormatRetentionSettings(settings retention.Settings) map[string]string {
	return opspkg.FormatRetentionSettings(settings)
}

func executeCommand(job terminal.CommandJob) terminal.CommandResult {
	return opspkg.ExecuteCommand(job)
}

func loadCommandExecutorConfig() commandExecutorConfig {
	return opspkg.LoadCommandExecutorConfig()
}

func executeConfiguredCommand(job terminal.CommandJob, cfg commandExecutorConfig) (string, string) {
	return opspkg.ExecuteConfiguredCommand(job, cfg)
}

func executeSSHCommand(job terminal.CommandJob, cfg commandExecutorConfig) (string, error) {
	return opspkg.ExecuteSSHCommand(job, cfg)
}

func executeLocalCommand(job terminal.CommandJob, cfg commandExecutorConfig) (string, error) {
	return opspkg.ExecuteLocalCommand(job, cfg)
}

func executeSimulatedCommand(job terminal.CommandJob) (string, string) {
	return opspkg.ExecuteSimulatedCommand(job)
}

func resolveJobSSHConfig(job terminal.CommandJob) (*terminal.SSHConfig, error) {
	return opspkg.ResolveJobSSHConfig(job)
}

func buildSSHHostKeyCallback(cfg *terminal.SSHConfig) (ssh.HostKeyCallback, error) {
	return opspkg.BuildSSHHostKeyCallback(cfg)
}

func resolveSSHAuthMethods(cfg *terminal.SSHConfig) ([]ssh.AuthMethod, error) {
	return opspkg.ResolveSSHAuthMethods(cfg)
}

func parseSSHTarget(raw, defaultUser string, defaultPort int) (sshTarget, error) {
	return opspkg.ParseSSHTarget(raw, defaultUser, defaultPort)
}

func truncateOutput(payload []byte, maxBytes int) string {
	return opspkg.TruncateOutput(payload, maxBytes)
}

func outputForError(output string, err error) string {
	return opspkg.OutputForError(output, err)
}

// Type aliases for executor types.
type updateScopeExecutor = opspkg.UpdateScopeExecutor

// Action and update executor forwarding.
func executeActionInProcess(job actions.Job, registry *connectorsdk.Registry) actions.Result {
	return opspkg.ExecuteActionInProcess(job, registry)
}

func executeUpdate(job updates.Job) updates.Result {
	return opspkg.ExecuteUpdate(job)
}

func executeUpdateWithExecutor(job updates.Job, executor updateScopeExecutor) updates.Result {
	return opspkg.ExecuteUpdateWithExecutor(job, executor)
}

func heartbeatLoop(ctx context.Context) {
	opspkg.HeartbeatLoop(ctx)
}

// Constants aliases.
const (
	executorModeSimulated = opspkg.ExecutorModeSimulated
	executorModeSSH       = opspkg.ExecutorModeSSH
	executorModeLocal     = opspkg.ExecutorModeLocal
)

// --- Policy runtime state aliases ---

type policyRuntimeState = opspkg.PolicyRuntimeState

func newPolicyRuntimeState(base policy.EvaluatorConfig) *policyRuntimeState {
	return opspkg.NewPolicyRuntimeState(base)
}

func loadPolicyConfigFromEnv() policy.EvaluatorConfig {
	return opspkg.LoadPolicyConfigFromEnv()
}

func refreshPolicyRuntimeSettingsDirect(ctx context.Context, store persistence.RuntimeSettingsStore, state *policyRuntimeState) {
	opspkg.RefreshPolicyRuntimeSettingsDirect(ctx, store, state)
}

func parseBoolFallback(value string, fallback bool) bool {
	return opspkg.ParseBoolFallback(value, fallback)
}

func parseCSVOrDefault(value string, fallback []string) []string {
	return opspkg.ParseCSVOrDefault(value, fallback)
}

// --- Security runtime state aliases ---

func buildSecurityRuntimeEnvOverrides(overrides map[string]string) map[string]string {
	return opspkg.BuildSecurityRuntimeEnvOverrides(overrides)
}

func applySecurityRuntimeOverrides(overrides map[string]string) {
	opspkg.ApplySecurityRuntimeOverrides(overrides)
}

func refreshSecurityRuntimeSettingsDirect(ctx context.Context, store persistence.RuntimeSettingsStore) {
	opspkg.RefreshSecurityRuntimeSettingsDirect(ctx, store)
}

// --- TLS runtime aliases ---

type hubCertificateSwitcher = opspkg.HubCertificateSwitcher
type staticHubCertificateProvider = opspkg.StaticHubCertificateProvider
type tlsCertificateMetadata = opspkg.TLSCertificateMetadata
type tlsOverrideMaterial = opspkg.TLSOverrideMaterial

func newStaticHubCertificateProvider(certFile, keyFile string) (*staticHubCertificateProvider, error) {
	return opspkg.NewStaticHubCertificateProvider(certFile, keyFile)
}

func tlsCertificateMetadataFromPEM(certPEM string) (tlsCertificateMetadata, error) {
	return opspkg.TLSCertificateMetadataFromPEM(certPEM)
}

func tlsCertificateMetadataFromFile(certFile string) (tlsCertificateMetadata, error) {
	return opspkg.TLSCertificateMetadataFromFile(certFile)
}

func validateUploadedTLSPair(certPEM, keyPEM string) (tlsCertificateMetadata, error) {
	return opspkg.ValidateUploadedTLSPair(certPEM, keyPEM)
}

func materializeUploadedTLSFiles(dataDir, certPEM, keyPEM string) (string, string, error) {
	return opspkg.MaterializeUploadedTLSFiles(dataDir, certPEM, keyPEM)
}

func loadPersistedTLSOverride(store interface {
	ListRuntimeSettingOverrides() (map[string]string, error)
}, secretsManager *secrets.Manager) (tlsOverrideMaterial, bool, error) {
	return opspkg.LoadPersistedTLSOverride(store, secretsManager)
}

func buildPinnedBootstrapURL(hubURL string, caCertPEM []byte) string {
	return opspkg.BuildPinnedBootstrapURL(hubURL, caCertPEM)
}

// --- Exec type aliases and forwarding ---

// v2ExecResult is a type alias so callers in cmd/labtether keep compiling
// without importing the operations package directly.
type v2ExecResult = opspkg.ExecResult

// v2ExecOnAsset is a thin stub so callers in apiv2_sysinfo.go, bulk_bridge.go,
// and actions_bridge.go keep compiling after the implementation moved to ExecDeps.
func (s *apiServer) v2ExecOnAsset(r *http.Request, assetID, command string, timeoutSec int) v2ExecResult {
	return s.ensureOperationsDeps().ExecOnAsset(r, assetID, command, timeoutSec)
}

// --- Exec handler deps wiring ---

// buildOperationsDeps constructs the operations.ExecDeps from apiServer fields.
func (s *apiServer) buildOperationsDeps() *opspkg.ExecDeps {
	return &opspkg.ExecDeps{
		AgentMgr:   s.agentMgr,
		AssetStore: s.assetStore,
		ExecuteViaAgent: func(job terminal.CommandJob) terminal.CommandResult {
			return s.executeViaAgent(job)
		},
		DecodeJSONBody: func(w http.ResponseWriter, r *http.Request, dst any) error {
			return shared.DecodeJSONBody(w, r, dst)
		},
		PrincipalActorID: func(ctx context.Context) string {
			return principalActorID(ctx)
		},
		AppendAuditEventBestEffort: func(event audit.Event, logMessage string) {
			s.appendAuditEventBestEffort(event, logMessage)
		},
		AllowedAssetsFromContext: func(ctx context.Context) []string {
			return allowedAssetsFromContext(ctx)
		},
		ScopesFromContext: func(ctx context.Context) []string {
			return scopesFromContext(ctx)
		},
		EnforceRateLimit: s.enforceRateLimit,
	}
}

// ensureOperationsDeps returns the operations deps, creating and caching on first call.
func (s *apiServer) ensureOperationsDeps() *opspkg.ExecDeps {
	s.operationsDepsOnce.Do(func() {
		s.operationsDeps = s.buildOperationsDeps()
	})
	return s.operationsDeps
}

// buildUpdateExecutorDeps constructs the UpdateExecutorDeps from apiServer fields.
func (s *apiServer) buildUpdateExecutorDeps() *opspkg.UpdateExecutorDeps {
	return &opspkg.UpdateExecutorDeps{
		AgentMgr:              s.agentMgr,
		AssetStore:            s.assetStore,
		ExecuteUpdateViaAgent: s.executeUpdateViaAgent,
	}
}

// executeUpdateScope delegates update scope execution to operations.UpdateExecutorDeps.
func (s *apiServer) executeUpdateScope(job updates.Job, target, scope string) updates.RunResultEntry {
	return s.buildUpdateExecutorDeps().ExecuteUpdateScope(job, target, scope)
}

// summarizeUpdateOutput is a thin alias for test/agent bridge compatibility.
func summarizeUpdateOutput(output string) string {
	return opspkg.SummarizeUpdateOutput(output)
}

// defaultUpdateAgentTimeout is an alias kept for agents_bridge.go.
const defaultUpdateAgentTimeout = opspkg.DefaultUpdateAgentTimeout

// TLS constants aliases.
const (
	tlsSourceDisabled           = opspkg.TLSSourceDisabled
	tlsSourceBuiltIn            = opspkg.TLSSourceBuiltIn
	tlsSourceTailscale          = opspkg.TLSSourceTailscale
	tlsSourceDeploymentExternal = opspkg.TLSSourceDeploymentExternal
	tlsSourceUIUploaded         = opspkg.TLSSourceUIUploaded

	tlsTrustModePublicTLS   = opspkg.TLSTrustModePublicTLS
	tlsTrustModeCustomTLS   = opspkg.TLSTrustModeCustomTLS
	tlsTrustModeLabtetherCA = opspkg.TLSTrustModeLabtetherCA
	tlsTrustModePlainHTTP   = opspkg.TLSTrustModePlainHTTP

	tlsBootstrapStrategyInstall  = opspkg.TLSBootstrapStrategyInstall
	tlsBootstrapStrategyPinnedCA = opspkg.TLSBootstrapStrategyPinnedCA

	tlsOverrideCertPEMKey   = opspkg.TLSOverrideCertPEMKey
	tlsOverrideKeyCipherKey = opspkg.TLSOverrideKeyCipherKey
	tlsOverrideUpdatedAtKey = opspkg.TLSOverrideUpdatedAtKey
	tlsOverrideAAD          = opspkg.TLSOverrideAAD

	tlsUploadedCertRelativePath = opspkg.TLSUploadedCertRelativePath
	tlsUploadedKeyRelativePath  = opspkg.TLSUploadedKeyRelativePath
)

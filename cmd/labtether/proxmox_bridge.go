package main

import (
	"context"
	"crypto/tls"
	"net/http"
	"time"

	"github.com/labtether/labtether/internal/actions"
	proxmoxconnector "github.com/labtether/labtether/internal/connectors/proxmox"
	"github.com/labtether/labtether/internal/connectorsdk"
	proxmoxpkg "github.com/labtether/labtether/internal/hubapi/proxmox"
	"github.com/labtether/labtether/internal/terminal"
)

// buildProxmoxDeps constructs the proxmox.Deps from the apiServer's fields.
func (s *apiServer) buildProxmoxDeps() *proxmoxpkg.Deps {
	return &proxmoxpkg.Deps{
		AssetStore:        s.assetStore,
		HubCollectorStore: s.hubCollectorStore,
		CredentialStore:   s.credentialStore,
		SecretsManager:    s.secretsManager,
		TelemetryStore:    s.telemetryStore,
		ConnectorRegistry: s.connectorRegistry,

		RequireAdminAuth: s.requireAdminAuth,
		EnforceRateLimit: s.enforceRateLimit,
		IssueStreamTicket: s.issueStreamTicket,

		SetDesktopSPICEProxyTarget: func(sessionID string, target proxmoxpkg.DesktopSPICEProxyTarget) {
			s.setDesktopSPICEProxyTarget(sessionID, desktopSPICEProxyTarget{
				Host:       target.Host,
				TLSPort:    target.TLSPort,
				Password:   target.Password,
				Type:       target.Type,
				CA:         target.CA,
				Proxy:      target.Proxy,
				SkipVerify: target.SkipVerify,
			})
		},
		TakeDesktopSPICEProxyTarget: func(sessionID string) (proxmoxpkg.DesktopSPICEProxyTarget, bool) {
			t, ok := s.takeDesktopSPICEProxyTarget(sessionID)
			return proxmoxpkg.DesktopSPICEProxyTarget{
				Host:       t.Host,
				TLSPort:    t.TLSPort,
				Password:   t.Password,
				Type:       t.Type,
				CA:         t.CA,
				Proxy:      t.Proxy,
				SkipVerify: t.SkipVerify,
			}, ok
		},

		ExecuteCommandAction: s.executeCommandAction,
		ExecuteActionInProcessFn: func(job actions.Job, registry *connectorsdk.Registry) actions.Result {
			return executeActionInProcess(job, registry)
		},

		TerminalWebSocketUpgrader: &terminalWebSocketUpgrader,
		MaxDesktopInputReadBytes:  maxDesktopInputReadBytes,

		WrapAuth:  s.withAuth,
		WrapAdmin: s.withAdminAuth,
	}
}

// Type aliases for proxmox types used in cmd/labtether/.
type proxmoxSessionTarget = proxmoxpkg.ProxmoxSessionTarget
type proxmoxRuntime = proxmoxpkg.ProxmoxRuntime
type cachedProxmoxRuntime = proxmoxpkg.CachedProxmoxRuntime
type proxmoxActionExecution = proxmoxpkg.ProxmoxActionExecution

// ensureProxmoxDeps returns proxmoxDeps. When pre-initialized (production),
// returns the cached instance for shared client caching. Otherwise, rebuilds
// on every call so that test mutations to apiServer fields are visible.
func (s *apiServer) ensureProxmoxDeps() *proxmoxpkg.Deps {
	if s.proxmoxDeps != nil {
		return s.proxmoxDeps
	}
	return s.buildProxmoxDeps()
}

// Forwarding methods from apiServer to proxmox.Deps so that existing
// cmd/labtether/ callers keep compiling without changes.

func (s *apiServer) handleProxmoxAssets(w http.ResponseWriter, r *http.Request) {
	s.ensureProxmoxDeps().HandleProxmoxAssets(w, r)
}

func (s *apiServer) handleProxmoxTaskRoutes(w http.ResponseWriter, r *http.Request) {
	s.ensureProxmoxDeps().HandleProxmoxTaskRoutes(w, r)
}

func (s *apiServer) handleProxmoxClusterStatus(w http.ResponseWriter, r *http.Request) {
	s.ensureProxmoxDeps().HandleProxmoxClusterStatus(w, r)
}

func (s *apiServer) handleProxmoxClusterResources(w http.ResponseWriter, r *http.Request) {
	s.ensureProxmoxDeps().HandleProxmoxClusterResources(w, r)
}

func (s *apiServer) handleProxmoxNodeRoutes(w http.ResponseWriter, r *http.Request) {
	s.ensureProxmoxDeps().HandleProxmoxNodeRoutes(w, r)
}

func (s *apiServer) handleProxmoxTaskLog(w http.ResponseWriter, r *http.Request) {
	s.ensureProxmoxDeps().HandleProxmoxTaskLog(w, r)
}

func (s *apiServer) handleProxmoxTaskStop(w http.ResponseWriter, r *http.Request) {
	s.ensureProxmoxDeps().HandleProxmoxTaskStop(w, r)
}

func (s *apiServer) handleProxmoxCephStatus(w http.ResponseWriter, r *http.Request) {
	s.ensureProxmoxDeps().HandleProxmoxCephStatus(w, r)
}

func (s *apiServer) handleDesktopSPICETicket(w http.ResponseWriter, r *http.Request, session terminal.Session) {
	s.ensureProxmoxDeps().HandleDesktopSPICETicket(w, r, session)
}

func (s *apiServer) handleDesktopSPICEStream(w http.ResponseWriter, r *http.Request, session terminal.Session) {
	s.ensureProxmoxDeps().HandleDesktopSPICEStream(w, r, session)
}

func (s *apiServer) handleProxmoxDesktopStream(w http.ResponseWriter, r *http.Request, session terminal.Session, target proxmoxpkg.ProxmoxSessionTarget) {
	s.ensureProxmoxDeps().HandleProxmoxDesktopStream(w, r, session, target)
}

func (s *apiServer) tryProxmoxTerminalStream(w http.ResponseWriter, r *http.Request, session terminal.Session, target proxmoxpkg.ProxmoxSessionTarget) error {
	return s.ensureProxmoxDeps().TryProxmoxTerminalStream(w, r, session, target)
}

func (s *apiServer) executeActionInProcess(job actions.Job) actions.Result {
	return s.ensureProxmoxDeps().ExecuteActionInProcess(job)
}

func (s *apiServer) executeProxmoxAction(ctx context.Context, actionID, target string, params map[string]string, dryRun bool) (proxmoxActionExecution, error) {
	return s.ensureProxmoxDeps().ExecuteProxmoxAction(ctx, actionID, target, params, dryRun)
}

func (s *apiServer) executeProxmoxActionDirect(ctx context.Context, actionID string, req connectorsdk.ActionRequest) (connectorsdk.ActionResult, error) {
	return s.ensureProxmoxDeps().ExecuteProxmoxActionDirect(ctx, actionID, req)
}

func (s *apiServer) resolveProxmoxSessionTarget(assetID string) (proxmoxpkg.ProxmoxSessionTarget, bool, error) {
	return s.ensureProxmoxDeps().ResolveProxmoxSessionTarget(assetID)
}

func (s *apiServer) loadProxmoxRuntime(collectorID string) (*proxmoxpkg.ProxmoxRuntime, error) {
	return s.ensureProxmoxDeps().LoadProxmoxRuntime(collectorID)
}

// Utility function aliases delegating to the proxmox package.

func (s *apiServer) resolveProxmoxActionTarget(actionID, target string) (string, string, string, error) {
	return s.ensureProxmoxDeps().ResolveProxmoxActionTarget(actionID, target)
}

func proxmoxActionErrorMessage(err error) string       { return proxmoxpkg.ProxmoxActionErrorMessage(err) }
func proxmoxActionOutput(result proxmoxActionExecution) string { return proxmoxpkg.ProxmoxActionOutput(result) }
func validateResolvedProxmoxActionTarget(resolved proxmoxSessionTarget, expectedKind, actionID string) error {
	return proxmoxpkg.ValidateResolvedProxmoxActionTarget(resolved, expectedKind, actionID)
}
func sortProxmoxSnapshots(snapshots []proxmoxconnector.Snapshot) []proxmoxconnector.Snapshot {
	return proxmoxpkg.SortProxmoxSnapshots(snapshots)
}
func filterAndSortProxmoxTasks(tasks []proxmoxconnector.Task, node, vmid string, limit int) []proxmoxconnector.Task {
	return proxmoxpkg.FilterAndSortProxmoxTasks(tasks, node, vmid, limit)
}

func selectProxmoxHA(resources []proxmoxconnector.HAResource, target proxmoxSessionTarget) (*proxmoxconnector.HAResource, []proxmoxconnector.HAResource) {
	return proxmoxpkg.SelectProxmoxHA(resources, target)
}

func buildProxmoxCapabilityTabs(kind string) []string {
	return proxmoxpkg.BuildProxmoxCapabilityTabs(kind)
}

func proxmoxSPICEOpenErrorResponse(err error) (int, string) {
	return proxmoxpkg.ProxmoxSPICEOpenErrorResponse(err)
}

func newProxmoxTLSConfig(skipVerify bool, caPEM string) (*tls.Config, error) {
	return proxmoxpkg.NewProxmoxTLSConfig(skipVerify, caPEM)
}

type proxmoxStoragePoolState = proxmoxpkg.ProxmoxStoragePoolState

func parseStorageInsightsWindow(raw string) time.Duration { return proxmoxpkg.ParseStorageInsightsWindow(raw) }
func clampPercent(value float64) float64                  { return proxmoxpkg.ClampPercent(value) }
func buildProxmoxStorageInsightEvents(tasks []proxmoxconnector.Task, poolStates []proxmoxStoragePoolState, now time.Time, window time.Duration) []proxmoxpkg.ProxmoxStorageInsightEvent {
	return proxmoxpkg.BuildProxmoxStorageInsightEvents(tasks, poolStates, now, window)
}

func (s *apiServer) loadProxmoxAssetDetails(ctx context.Context, assetID string, target proxmoxSessionTarget, runtime *proxmoxRuntime) (proxmoxpkg.ProxmoxAssetDetailsResponse, error) {
	return s.ensureProxmoxDeps().LoadProxmoxAssetDetails(ctx, assetID, target, runtime)
}

func (s *apiServer) loadProxmoxStorageInsights(ctx context.Context, assetID string, target proxmoxSessionTarget, runtime *proxmoxRuntime, window time.Duration) (proxmoxpkg.ProxmoxStorageInsightsResponse, error) {
	return s.ensureProxmoxDeps().LoadProxmoxStorageInsights(ctx, assetID, target, runtime, window)
}
func parsePositiveInt(raw string) (int, bool)                   { return proxmoxpkg.ParsePositiveInt(raw) }
func parseAnyInt64(value any) (int64, bool)                     { return proxmoxpkg.ParseAnyInt64(value) }
func formatStorageInsightsWindow(window time.Duration) string   { return proxmoxpkg.FormatStorageInsightsWindow(window) }
func (s *apiServer) handleProxmoxNodeNetwork(w http.ResponseWriter, r *http.Request) { s.ensureProxmoxDeps().HandleProxmoxNodeNetwork(w, r) }
func (s *apiServer) openProxmoxTerminalTicket(ctx context.Context, runtime *proxmoxRuntime, target proxmoxSessionTarget) (proxmoxconnector.ProxyTicket, error) { return s.ensureProxmoxDeps().OpenProxmoxTerminalTicket(ctx, runtime, target) }

package main

import (
	"net/http"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/connectorsdk"
	opspkg "github.com/labtether/labtether/internal/hubapi/operations"
	proxmoxpkg "github.com/labtether/labtether/internal/hubapi/proxmox"
)

// buildProxmoxDeps constructs the proxmox.Deps from the apiServer's fields.
func (s *apiServer) buildProxmoxDeps() *proxmoxpkg.Deps {
	return &proxmoxpkg.Deps{
		AssetStore:                  s.assetStore,
		HubCollectorStore:           s.hubCollectorStore,
		CredentialStore:             s.credentialStore,
		SecretsManager:              s.secretsManager,
		TelemetryStore:              s.telemetryStore,
		ConnectorRegistry:           s.connectorRegistry,
		EvaluateConnectorGuardrails: s.connectorMaintenanceEvaluator(),
		ConfiguredConnectorActions:  s.configuredConnectorActions(),

		RequireAdminAuth:  s.requireAdminAuth,
		EnforceRateLimit:  s.enforceRateLimit,
		IssueStreamTicket: s.issueStreamTicket,

		SetDesktopSPICEProxyTarget: func(sessionID string, target proxmoxpkg.DesktopSPICEProxyTarget) {
			s.setDesktopSPICEProxyTarget(sessionID, desktopSPICEProxyTarget{
				Host:        target.Host,
				TLSPort:     target.TLSPort,
				Password:    target.Password,
				Type:        target.Type,
				CA:          target.CA,
				Proxy:       target.Proxy,
				HostSubject: target.HostSubject,
				SkipVerify:  target.SkipVerify,
			})
		},
		TakeDesktopSPICEProxyTarget: func(sessionID string) (proxmoxpkg.DesktopSPICEProxyTarget, bool) {
			t, ok := s.takeDesktopSPICEProxyTarget(sessionID)
			return proxmoxpkg.DesktopSPICEProxyTarget{
				Host:        t.Host,
				TLSPort:     t.TLSPort,
				Password:    t.Password,
				Type:        t.Type,
				CA:          t.CA,
				Proxy:       t.Proxy,
				HostSubject: t.HostSubject,
				SkipVerify:  t.SkipVerify,
			}, ok
		},

		ExecuteCommandAction:            s.executeCommandAction,
		ExecuteCommandActionContext:     s.executeCommandActionContext,
		ExecuteActionInProcessContextFn: opspkg.ExecuteActionInProcessContext,
		ExecuteActionInProcessFn: func(job actions.Job, registry *connectorsdk.Registry) actions.Result {
			return executeActionInProcess(job, registry)
		},

		TerminalWebSocketUpgrader: &terminalWebSocketUpgrader,
		MaxDesktopInputReadBytes:  maxDesktopInputReadBytes,

		WrapAuth:  s.withAuth,
		WrapAdmin: s.withAdminAuth,
	}
}

// ensureProxmoxDeps returns proxmoxDeps. When pre-initialized (production),
// returns the cached instance for shared client caching. Otherwise, rebuilds
// on every call so that test mutations to apiServer fields are visible.
//
// This function does NOT have the race-on-lazy-init pattern that
// ensureCollectorsDeps had, because neither branch mutates shared state: the
// cached branch is a pure read, and the rebuild branch returns a local value
// without assigning it to s.proxmoxDeps. Concurrent callers are safe.
func (s *apiServer) ensureProxmoxDeps() *proxmoxpkg.Deps {
	if s.proxmoxDeps != nil {
		return s.proxmoxDeps
	}
	return s.buildProxmoxDeps()
}

// The three handlers below stay in cmd: each refuses a connector write while
// Settings › Integration actions is off (refuseIntegrationActionWhenOff, cmd
// code) before the package handler runs. The v1 route and its v2 wrapper both
// call them, so the check is in one place. Every other Proxmox route and
// caller calls ensureProxmoxDeps() itself, when the request or call is made.

func (s *apiServer) handleProxmoxAssets(w http.ResponseWriter, r *http.Request) {
	if s.refuseIntegrationActionWhenOff(w, r) {
		return
	}
	s.ensureProxmoxDeps().HandleProxmoxAssets(w, r)
}

func (s *apiServer) handleProxmoxTaskRoutes(w http.ResponseWriter, r *http.Request) {
	if s.refuseIntegrationActionWhenOff(w, r) {
		return
	}
	s.ensureProxmoxDeps().HandleProxmoxTaskRoutes(w, r)
}

func (s *apiServer) handleProxmoxNodeRoutes(w http.ResponseWriter, r *http.Request) {
	if s.refuseIntegrationActionWhenOff(w, r) {
		return
	}
	s.ensureProxmoxDeps().HandleProxmoxNodeRoutes(w, r)
}

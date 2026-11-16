package hubdeps

import (
	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/hubapi/desktop"
	"github.com/labtether/labtether/internal/hubapi/operations"
	"github.com/labtether/labtether/internal/hubapi/proxmox"
	"github.com/labtether/labtether/internal/hubapi/testutil"
)

// maxDesktopInputReadBytes is cmd/labtether's constant of the same name.
const maxDesktopInputReadBytes = 256 * 1024

// BuildProxmoxDeps mirrors cmd's buildProxmoxDeps field by field. Stand-ins
// replace only cmd's middleware (route auth, admin check, rate limit, stream
// tickets, WebSocket origin policy) and agent command dispatch; see
// testutil/standins.go for what each one still refuses.
func (h *Hub) BuildProxmoxDeps() *proxmox.Deps {
	executeCommand, executeCommandContext := testutil.CommandDispatchNotTested(h.t)
	spice := h.DesktopSPICEDeps()
	return &proxmox.Deps{
		AssetStore:                  h.AssetStore,
		HubCollectorStore:           h.HubCollectorStore,
		CredentialStore:             h.CredentialStore,
		SecretsManager:              h.SecretsManager,
		TelemetryStore:              h.TelemetryStore,
		ConnectorRegistry:           h.ConnectorRegistry,
		EvaluateConnectorGuardrails: h.ConnectorMaintenanceEvaluator(),
		ConfiguredConnectorActions:  h.ConfiguredConnectorActions(),

		RequireAdminAuth:  testutil.RequireAdminRole,
		EnforceRateLimit:  h.RateLimiter.Enforce,
		IssueStreamTicket: h.StreamTickets.Issue,

		SetDesktopSPICEProxyTarget: func(sessionID string, target proxmox.DesktopSPICEProxyTarget) {
			spice.SetDesktopSPICEProxyTarget(sessionID, desktop.DesktopSPICEProxyTarget{
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
		TakeDesktopSPICEProxyTarget: func(sessionID string) (proxmox.DesktopSPICEProxyTarget, bool) {
			t, ok := spice.TakeDesktopSPICEProxyTarget(sessionID)
			return proxmox.DesktopSPICEProxyTarget{
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

		ExecuteCommandAction:            executeCommand,
		ExecuteCommandActionContext:     executeCommandContext,
		ExecuteActionInProcessContextFn: operations.ExecuteActionInProcessContext,
		ExecuteActionInProcessFn: func(job actions.Job, registry *connectorsdk.Registry) actions.Result {
			return operations.ExecuteActionInProcess(job, registry)
		},

		TerminalWebSocketUpgrader: testutil.NewStreamUpgrader(),
		MaxDesktopInputReadBytes:  maxDesktopInputReadBytes,

		WrapAuth:  testutil.RouteAuthNotTested(h.t),
		WrapAdmin: testutil.RouteAuthNotTested(h.t),
	}
}

// ProxmoxDeps mirrors cmd's ensureProxmoxDeps: the pinned Deps when one is
// set, otherwise a fresh build, so store changes made by a test are seen.
func (h *Hub) ProxmoxDeps() *proxmox.Deps {
	if h.Proxmox != nil {
		return h.Proxmox
	}
	return h.BuildProxmoxDeps()
}

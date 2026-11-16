package hubdeps

import (
	"context"
	"net/http"
	"sync"
	"testing"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/hubapi/desktop"
	"github.com/labtether/labtether/internal/hubapi/proxmox"
	"github.com/labtether/labtether/internal/hubapi/shared"
	terminalpkg "github.com/labtether/labtether/internal/hubapi/terminal"
	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/protocols"
	"github.com/labtether/labtether/internal/terminal"
)

// maxTargetLength is cmd/labtether's constant of the same name.
const maxTargetLength = 255

// BuildDesktopDeps mirrors cmd's buildDesktopDeps field by field. Stand-ins
// replace only cmd's middleware (route auth, rate limit, stream tickets and
// the WebSocket origin check); see testutil/standins.go for what each one
// still refuses. Like cmd's test server, the Hub has no database: no DB pool,
// no protocol configs, and recordings start without a file.
func (h *Hub) BuildDesktopDeps() *desktop.Deps {
	return &desktop.Deps{
		SessionStreams:  &h.SessionStreams,
		TerminalStore:   h.TerminalStore,
		AssetStore:      h.AssetStore,
		CredentialStore: h.CredentialStore,

		DBPool:        nil,
		RecordingsDir: desktop.RecordingsDirForDataDir(""),

		AgentMgr:       h.AgentMgr,
		SecretsManager: h.SecretsManager,
		PolicyState:    h.PolicyState,

		DesktopBridges:    &h.DesktopBridges,
		WebRTCBridges:     &h.WebRTCBridges,
		ClipboardReplies:  &h.ClipboardReplies,
		DisplayReplies:    &h.DisplayReplies,
		DiagnosticReplies: &h.DiagnosticReplies,

		DesktopSessionMu:   &h.desktopSession.mu,
		DesktopSessionOpts: &h.desktopSession.opts,
		DesktopSPICEMu:     &h.desktopSPICE.mu,
		DesktopSPICE:       &h.desktopSPICE.targets,

		TerminalWebSocketUpgrader: testutil.NewStreamUpgrader(),
		MaxDesktopInputReadBytes:  maxDesktopInputReadBytes,
		MaxTargetLength:           maxTargetLength,

		EnforceRateLimit: h.RateLimiter.Enforce,
		WrapAuth:         testutil.RouteAuthNotTested(h.t),

		IssueStreamTicket:     h.StreamTickets.Issue,
		DecodeJSONBody:        shared.DecodeJSONBody,
		ValidateMaxLen:        shared.ValidateMaxLen,
		PrincipalActorID:      apiv2.PrincipalActorID,
		CanAccessOwnedSession: sessionOwnerCheckNotTested(h.t),
		// cmd's generateRequestID and sanitizeUpstreamError reach these
		// through the resources and collectors packages.
		GenerateRequestID:          shared.GenerateRequestID,
		SanitizeUpstreamError:      shared.SanitizeUpstreamError,
		EnvOrDefaultBool:           shared.EnvOrDefaultBool,
		BrowserStreamTraceID:       shared.BrowserStreamTraceID,
		StreamTraceLogValue:        shared.StreamTraceLogValue,
		SanitizeAgentStreamReason:  terminalpkg.SanitizeAgentStreamReason,
		StartBrowserWSKeepalive:    shared.StartBrowserWebSocketKeepalive,
		TouchBrowserWSReadDeadline: shared.TouchBrowserWebSocketReadDeadline,
		UserIDFromContext:          apiv2.UserIDFromContext,
		UserRoleFromContext:        apiv2.UserRoleFromContext,
		CheckSameOrigin:            testutil.CheckNoBrowserOrigin,
		EvaluateAssetGuardrails:    h.GroupFeaturesDeps().EvaluateAssetGuardrails,

		GetProtocolConfig: func(context.Context, string, string) (*protocols.ProtocolConfig, error) {
			return nil, nil // cmd's closure answers this when it has no database
		},

		ResolveProxmoxSessionTarget: func(assetID string) (any, bool, error) {
			return h.ProxmoxDeps().ResolveProxmoxSessionTarget(assetID)
		},
		HandleProxmoxDesktopStream: func(w http.ResponseWriter, r *http.Request, session terminal.Session, target any) {
			h.ProxmoxDeps().HandleProxmoxDesktopStream(w, r, session, target.(proxmox.ProxmoxSessionTarget))
		},
		HandleDesktopSPICETicket: func(w http.ResponseWriter, r *http.Request, session terminal.Session) {
			h.ProxmoxDeps().HandleDesktopSPICETicket(w, r, session)
		},
		HandleDesktopSPICEStream: func(w http.ResponseWriter, r *http.Request, session terminal.Session) {
			h.ProxmoxDeps().HandleDesktopSPICEStream(w, r, session)
		},

		// cmd's startRecording and stopRecording without a database.
		StartRecording: func(string, string, string, string) (*desktop.ActiveRecording, error) {
			return nil, nil
		},
		StopRecording: func(rec *desktop.ActiveRecording) {
			if rec != nil {
				rec.Stop()
			}
		},
	}
}

// DesktopDeps mirrors cmd's ensureDesktopDeps: built on first use and kept,
// so the bridge owners and session options it holds last for the test.
// ResetDesktopDeps makes the next call build again.
func (h *Hub) DesktopDeps() *desktop.Deps {
	h.desktopOnce.Do(func() { h.desktopDeps = h.BuildDesktopDeps() })
	return h.desktopDeps
}

// ResetDesktopDeps mirrors cmd's resetDesktopDepsForTest.
func (h *Hub) ResetDesktopDeps() {
	h.desktopDeps = nil
	h.desktopOnce = sync.Once{}
}

// sessionOwnerCheckNotTested stands in for cmd's canAccessOwnedSession, which
// decides who may open another user's session. It fails the test if a
// request reaches it and refuses: a copy here could drift from cmd's rule
// and pass a test cmd would refuse. That rule is tested in cmd.
func sessionOwnerCheckNotTested(t *testing.T) func(*http.Request, string) bool {
	return func(*http.Request, string) bool {
		t.Errorf("a request reached the session owner check stand-in; that check is tested in cmd/labtether")
		return false
	}
}

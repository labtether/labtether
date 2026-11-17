package main

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/apiv2"
	collectorspkg "github.com/labtether/labtether/internal/hubapi/collectors"
	desktoppkg "github.com/labtether/labtether/internal/hubapi/desktop"
	proxmoxpkg "github.com/labtether/labtether/internal/hubapi/proxmox"
	respkg "github.com/labtether/labtether/internal/hubapi/resources"
	"github.com/labtether/labtether/internal/hubapi/shared"
	terminalpkg "github.com/labtether/labtether/internal/hubapi/terminal"
	"github.com/labtether/labtether/internal/protocols"
	"github.com/labtether/labtether/internal/terminal"
)

// buildDesktopDeps constructs the desktop.Deps from the apiServer's fields.
func (s *apiServer) buildDesktopDeps() *desktoppkg.Deps {
	return &desktoppkg.Deps{
		SessionStreams:  &s.sessionStreams,
		TerminalStore:   s.terminalStore,
		AssetStore:      s.assetStore,
		CredentialStore: s.credentialStore,

		DBPool:        dbPoolOrNil(s.db),
		RecordingsDir: s.recordingsDir(),

		AgentMgr:       s.agentMgr,
		SecretsManager: s.secretsManager,
		PolicyState:    s.policyState,

		DesktopBridges:    &s.desktopBridges,
		WebRTCBridges:     &s.webrtcBridges,
		ClipboardReplies:  &s.clipboardReplies,
		DisplayReplies:    &s.displayReplies,
		DiagnosticReplies: &s.diagnosticReplies,

		DesktopSessionMu:   &s.desktopSessionMu,
		DesktopSessionOpts: &s.desktopSessionOpts,
		DesktopSPICEMu:     &s.desktopSPICEMu,
		DesktopSPICE:       &s.desktopSPICE,

		TerminalWebSocketUpgrader: &terminalWebSocketUpgrader,
		MaxDesktopInputReadBytes:  maxDesktopInputReadBytes,
		MaxTargetLength:           maxTargetLength,

		EnforceRateLimit: s.enforceRateLimit,
		WrapAuth:         s.withAuth,

		IssueStreamTicket: func(ctx context.Context, sessionID string) (string, time.Time, error) {
			return s.issueStreamTicket(ctx, sessionID)
		},
		DecodeJSONBody: func(w http.ResponseWriter, r *http.Request, dst any) error {
			return shared.DecodeJSONBody(w, r, dst)
		},
		ValidateMaxLen: func(field, value string, maxLen int) error {
			return shared.ValidateMaxLen(field, value, maxLen)
		},
		PrincipalActorID: func(ctx context.Context) string {
			return apiv2.PrincipalActorID(ctx)
		},
		CanAccessOwnedSession: func(r *http.Request, sessionActorID string) bool {
			return canAccessOwnedSession(r, sessionActorID)
		},
		GenerateRequestID: func() string {
			return respkg.GenerateRequestID()
		},
		SanitizeUpstreamError: func(msg string) string {
			return collectorspkg.SanitizeUpstreamError(msg)
		},
		EnvOrDefaultBool: func(key string, fallback bool) bool {
			return shared.EnvOrDefaultBool(key, fallback)
		},
		BrowserStreamTraceID: func(r *http.Request) string {
			return shared.BrowserStreamTraceID(r)
		},
		StreamTraceLogValue: func(traceID string) string {
			return shared.StreamTraceLogValue(traceID)
		},
		SanitizeAgentStreamReason: func(raw string) string {
			return terminalpkg.SanitizeAgentStreamReason(raw)
		},
		StartBrowserWSKeepalive: func(wsConn *websocket.Conn, writeMu *sync.Mutex, streamLabel string) func() {
			return shared.StartBrowserWebSocketKeepalive(wsConn, writeMu, streamLabel)
		},
		TouchBrowserWSReadDeadline: func(wsConn *websocket.Conn) error {
			return shared.TouchBrowserWebSocketReadDeadline(wsConn)
		},
		UserIDFromContext: func(ctx context.Context) string {
			return apiv2.UserIDFromContext(ctx)
		},
		UserRoleFromContext: func(ctx context.Context) string {
			return apiv2.UserRoleFromContext(ctx)
		},
		CheckSameOrigin: func(r *http.Request) bool {
			return checkSameOrigin(r)
		},
		EvaluateAssetGuardrails: s.ensureGroupFeaturesDeps().EvaluateAssetGuardrails,

		GetProtocolConfig: func(ctx context.Context, assetID, protocol string) (*protocols.ProtocolConfig, error) {
			if s.db == nil {
				return nil, nil
			}
			return s.db.GetProtocolConfig(ctx, assetID, protocol)
		},

		ResolveProxmoxSessionTarget: func(assetID string) (any, bool, error) {
			return s.ensureProxmoxDeps().ResolveProxmoxSessionTarget(assetID)
		},
		HandleProxmoxDesktopStream: func(w http.ResponseWriter, r *http.Request, session terminal.Session, target any) {
			s.ensureProxmoxDeps().HandleProxmoxDesktopStream(w, r, session, target.(proxmoxpkg.ProxmoxSessionTarget))
		},
		HandleDesktopSPICETicket: func(w http.ResponseWriter, r *http.Request, session terminal.Session) {
			s.ensureProxmoxDeps().HandleDesktopSPICETicket(w, r, session)
		},
		HandleDesktopSPICEStream: func(w http.ResponseWriter, r *http.Request, session terminal.Session) {
			s.ensureProxmoxDeps().HandleDesktopSPICEStream(w, r, session)
		},

		StartRecording: func(sessionID, assetID, actorID, protocol string) (*desktoppkg.ActiveRecording, error) {
			return s.startRecording(sessionID, assetID, actorID, protocol)
		},
		StopRecording: func(rec *desktoppkg.ActiveRecording) {
			s.stopRecording(rec)
		},
	}
}

// ensureDesktopDeps returns the desktop deps, creating and caching on first call.
func (s *apiServer) ensureDesktopDeps() *desktoppkg.Deps {
	s.desktopDepsOnce.Do(func() {
		s.desktopDeps = s.buildDesktopDeps()
	})
	return s.desktopDeps
}

// Routes, the agent router and other cmd code call the desktop package
// through ensureDesktopDeps. The recording helpers below stay: they are the
// builder's callbacks and read cmd's database and data folder.

func (s *apiServer) startRecording(sessionID, assetID, actorID, protocol string) (*desktoppkg.ActiveRecording, error) {
	if s.db == nil {
		return nil, nil
	}
	return desktoppkg.DefaultStartRecording(s.db.Pool(), s.recordingsDir(), sessionID, assetID, actorID, protocol)
}

// recordingsDir keeps recordings on the persistent data volume.
func (s *apiServer) recordingsDir() string {
	return desktoppkg.RecordingsDirForDataDir(s.dataDir)
}

func (s *apiServer) stopRecording(rec *desktoppkg.ActiveRecording) {
	if rec == nil {
		return
	}
	if s.db == nil {
		rec.Stop()
		return
	}
	desktoppkg.DefaultStopRecording(s.db.Pool(), rec)
}

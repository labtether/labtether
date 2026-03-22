package main

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/agentmgr"
	desktoppkg "github.com/labtether/labtether/internal/hubapi/desktop"
	"github.com/labtether/labtether/internal/protocols"
	"github.com/labtether/labtether/internal/terminal"
)

// buildDesktopDeps constructs the desktop.Deps from the apiServer's fields.
func (s *apiServer) buildDesktopDeps() *desktoppkg.Deps {
	return &desktoppkg.Deps{
		TerminalStore:   s.terminalStore,
		AssetStore:      s.assetStore,
		CredentialStore: s.credentialStore,

		DBPool: dbPoolOrNil(s.db),

		AgentMgr:       s.agentMgr,
		SecretsManager: s.secretsManager,
		PolicyState:    s.policyState,

		DesktopBridges:           &s.desktopBridges,
		WebRTCBridges:            &s.webrtcBridges,
		ClipboardBridges:         &s.clipboardBridges,
		DisplayBridges:           &s.displayBridges,
		DesktopDiagnosticWaiters: &s.desktopDiagnosticWaiters,

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
			return decodeJSONBody(w, r, dst)
		},
		ValidateMaxLen: func(field, value string, maxLen int) error {
			return validateMaxLen(field, value, maxLen)
		},
		PrincipalActorID: func(ctx context.Context) string {
			return principalActorID(ctx)
		},
		IsOwnerActor: func(actorID string) bool {
			return isOwnerActor(actorID)
		},
		CanAccessOwnedSession: func(r *http.Request, sessionActorID string) bool {
			return canAccessOwnedSession(r, sessionActorID)
		},
		GenerateRequestID: func() string {
			return generateRequestID()
		},
		SanitizeUpstreamError: func(msg string) string {
			return sanitizeUpstreamError(msg)
		},
		EnvOrDefaultBool: func(key string, fallback bool) bool {
			return envOrDefaultBool(key, fallback)
		},
		BrowserStreamTraceID: func(r *http.Request) string {
			return browserStreamTraceID(r)
		},
		StreamTraceLogValue: func(traceID string) string {
			return streamTraceLogValue(traceID)
		},
		SanitizeAgentStreamReason: func(raw string) string {
			return sanitizeAgentStreamReason(raw)
		},
		StartBrowserWSKeepalive: func(wsConn *websocket.Conn, writeMu *sync.Mutex, streamLabel string) func() {
			return startBrowserWebSocketKeepalive(wsConn, writeMu, streamLabel)
		},
		TouchBrowserWSReadDeadline: func(wsConn *websocket.Conn) error {
			return touchBrowserWebSocketReadDeadline(wsConn)
		},
		UserIDFromContext: func(ctx context.Context) string {
			return userIDFromContext(ctx)
		},
		UserRoleFromContext: func(ctx context.Context) string {
			return userRoleFromContext(ctx)
		},
		CheckSameOrigin: func(r *http.Request) bool {
			return checkSameOrigin(r)
		},

		GetProtocolConfig: func(ctx context.Context, assetID, protocol string) (*protocols.ProtocolConfig, error) {
			if s.db == nil {
				return nil, nil
			}
			return s.db.GetProtocolConfig(ctx, assetID, protocol)
		},

		ResolveProxmoxSessionTarget: func(assetID string) (any, bool, error) {
			return s.resolveProxmoxSessionTarget(assetID)
		},
		HandleProxmoxDesktopStream: func(w http.ResponseWriter, r *http.Request, session terminal.Session, target any) {
			s.handleProxmoxDesktopStream(w, r, session, target.(proxmoxSessionTarget))
		},
		HandleDesktopSPICETicket: func(w http.ResponseWriter, r *http.Request, session terminal.Session) {
			s.handleDesktopSPICETicket(w, r, session)
		},
		HandleDesktopSPICEStream: func(w http.ResponseWriter, r *http.Request, session terminal.Session) {
			s.handleDesktopSPICEStream(w, r, session)
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
	if s.desktopDeps != nil {
		return s.desktopDeps
	}
	d := s.buildDesktopDeps()
	s.desktopDeps = d
	return d
}

// Forwarding methods from apiServer to desktop.Deps so that existing
// cmd/labtether/ callers keep compiling without changes.

func (s *apiServer) handleDesktopSessions(w http.ResponseWriter, r *http.Request) {
	s.ensureDesktopDeps().HandleDesktopSessions(w, r)
}

func (s *apiServer) handleDesktopSessionActions(w http.ResponseWriter, r *http.Request) {
	s.ensureDesktopDeps().HandleDesktopSessionActions(w, r)
}

func (s *apiServer) handleDesktopDiagnoseRequest(w http.ResponseWriter, r *http.Request) {
	s.ensureDesktopDeps().HandleDesktopDiagnoseRequest(w, r)
}

func (s *apiServer) handleRecordings(w http.ResponseWriter, r *http.Request) {
	s.ensureDesktopDeps().HandleRecordings(w, r)
}

func (s *apiServer) handleRecordingActions(w http.ResponseWriter, r *http.Request) {
	s.ensureDesktopDeps().HandleRecordingActions(w, r)
}

func (s *apiServer) handleNodeSubRoutes(w http.ResponseWriter, r *http.Request) {
	s.ensureDesktopDeps().HandleNodeSubRoutes(w, r)
}

func (s *apiServer) handleDesktopStream(w http.ResponseWriter, r *http.Request, session terminal.Session) {
	s.ensureDesktopDeps().HandleDesktopStream(w, r, session)
}

func (s *apiServer) handleDesktopStreamTicket(w http.ResponseWriter, r *http.Request, session terminal.Session) {
	s.ensureDesktopDeps().HandleDesktopStreamTicket(w, r, session)
}

func (s *apiServer) handleAgentDesktopStream(w http.ResponseWriter, r *http.Request, session terminal.Session) {
	s.ensureDesktopDeps().HandleAgentDesktopStream(w, r, session)
}

func (s *apiServer) handleDesktopAudioStream(w http.ResponseWriter, r *http.Request, session terminal.Session) {
	s.ensureDesktopDeps().HandleDesktopAudioStream(w, r, session)
}

func (s *apiServer) handleWebRTCStream(w http.ResponseWriter, r *http.Request, session terminal.Session) {
	s.ensureDesktopDeps().HandleWebRTCStream(w, r, session)
}

func (s *apiServer) handleGuacdDesktopStream(w http.ResponseWriter, r *http.Request, session terminal.Session) {
	s.ensureDesktopDeps().HandleGuacdDesktopStream(w, r, session)
}

func (s *apiServer) handleClipboardRoutes(w http.ResponseWriter, r *http.Request) {
	s.ensureDesktopDeps().HandleClipboardRoutes(w, r)
}

func (s *apiServer) handleDisplayList(w http.ResponseWriter, r *http.Request, assetID string) {
	// The display list is a private method on Deps but we need it from assets_groups_handlers.
	// Route through HandleNodeSubRoutes — or just forward directly via the display bridge approach.
	// Since handleDisplayList was exported from display_handlers (now in desktop pkg), we
	// construct a request path that HandleNodeSubRoutes can dispatch.
	// However, the simplest approach is to expose a thin wrapper.
	d := s.ensureDesktopDeps()
	d.HandleDisplayListDirect(w, r, assetID)
}

func (s *apiServer) processAgentDesktopStarted(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureDesktopDeps().ProcessAgentDesktopStarted(conn, msg)
}

func (s *apiServer) processAgentDesktopData(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureDesktopDeps().ProcessAgentDesktopData(conn, msg)
}

func (s *apiServer) processAgentDesktopClosed(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureDesktopDeps().ProcessAgentDesktopClosed(conn, msg)
}

func (s *apiServer) processAgentDesktopDisplays(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureDesktopDeps().ProcessAgentDesktopDisplays(conn, msg)
}

func (s *apiServer) processAgentDesktopAudioData(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureDesktopDeps().ProcessAgentDesktopAudioData(conn, msg)
}

func (s *apiServer) processAgentDesktopAudioState(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureDesktopDeps().ProcessAgentDesktopAudioState(conn, msg)
}

func (s *apiServer) processAgentWebRTCCapabilities(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureDesktopDeps().ProcessAgentWebRTCCapabilities(conn, msg)
}

func (s *apiServer) processAgentWebRTCStarted(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureDesktopDeps().ProcessAgentWebRTCStarted(conn, msg)
}

func (s *apiServer) processAgentWebRTCAnswer(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureDesktopDeps().ProcessAgentWebRTCAnswer(conn, msg)
}

func (s *apiServer) processAgentWebRTCICE(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureDesktopDeps().ProcessAgentWebRTCICE(conn, msg)
}

func (s *apiServer) processAgentWebRTCStopped(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureDesktopDeps().ProcessAgentWebRTCStopped(conn, msg)
}

func (s *apiServer) processAgentClipboardData(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureDesktopDeps().ProcessAgentClipboardData(conn, msg)
}

func (s *apiServer) processAgentClipboardSetAck(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureDesktopDeps().ProcessAgentClipboardSetAck(conn, msg)
}

func (s *apiServer) processAgentDesktopDiagnosed(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureDesktopDeps().ProcessAgentDesktopDiagnosed(conn, msg)
}

func (s *apiServer) closeDesktopBridgesForAsset(assetID string) {
	s.ensureDesktopDeps().CloseDesktopBridgesForAsset(assetID)
}

func (s *apiServer) closeWebRTCBridgesForAsset(assetID string) {
	s.ensureDesktopDeps().CloseWebRTCBridgesForAsset(assetID)
}

func (s *apiServer) finalizeAgentDesktopSession(sessionID string, bridgeState *desktoppkg.DesktopBridge, agentConn *agentmgr.AgentConn, startSent bool, closeFn func(*agentmgr.AgentConn, string)) {
	s.ensureDesktopDeps().FinalizeAgentDesktopSession(sessionID, bridgeState, agentConn, startSent, closeFn)
}

func (s *apiServer) setDesktopSessionOptions(sessionID string, opts desktoppkg.DesktopSessionOptions) {
	s.ensureDesktopDeps().SetDesktopSessionOptions(sessionID, opts)
}

func (s *apiServer) getDesktopSessionOptions(sessionID string) desktoppkg.DesktopSessionOptions {
	return s.ensureDesktopDeps().GetDesktopSessionOptions(sessionID)
}

func (s *apiServer) clearDesktopSessionOptions(sessionID string) {
	s.ensureDesktopDeps().ClearDesktopSessionOptions(sessionID)
}

func (s *apiServer) setDesktopSPICEProxyTarget(sessionID string, target desktoppkg.DesktopSPICEProxyTarget) {
	s.ensureDesktopDeps().SetDesktopSPICEProxyTarget(sessionID, target)
}

func (s *apiServer) takeDesktopSPICEProxyTarget(sessionID string) (desktoppkg.DesktopSPICEProxyTarget, bool) {
	return s.ensureDesktopDeps().TakeDesktopSPICEProxyTarget(sessionID)
}

func (s *apiServer) clearDesktopSPICEProxyTarget(sessionID string) {
	s.ensureDesktopDeps().ClearDesktopSPICEProxyTarget(sessionID)
}

func (s *apiServer) shouldUseWebRTC(assetID string) bool {
	return s.ensureDesktopDeps().ShouldUseWebRTC(assetID)
}

func (s *apiServer) resolveDesktopProtocol(session terminal.Session, r *http.Request) string {
	return s.ensureDesktopDeps().ResolveDesktopProtocol(session, r)
}

func (s *apiServer) bridgeDesktopOutput(wsConn *websocket.Conn, outputCh <-chan []byte, closedCh <-chan struct{}, writeMu *sync.Mutex, bridge *desktoppkg.DesktopBridge, logContext string) {
	s.ensureDesktopDeps().BridgeDesktopOutput(wsConn, outputCh, closedCh, writeMu, bridge, logContext)
}

func (s *apiServer) bridgeDesktopInput(wsConn *websocket.Conn, agentConn *agentmgr.AgentConn, sessionID string, closedCh <-chan struct{}, bridgeState *desktoppkg.DesktopBridge) (string, error) {
	return s.ensureDesktopDeps().BridgeDesktopInput(wsConn, agentConn, sessionID, closedCh, bridgeState)
}

func (s *apiServer) startRecording(sessionID, assetID, actorID, protocol string) (*desktoppkg.ActiveRecording, error) {
	if s.db == nil {
		return nil, nil
	}
	return desktoppkg.DefaultStartRecording(s.db.Pool(), sessionID, assetID, actorID, protocol)
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

func (s *apiServer) startRecordingRequest(w http.ResponseWriter, r *http.Request) {
	// startRecordingRequest is a POST handler within handleRecordings.
	// For test compatibility, route through HandleRecordings which dispatches POST.
	s.ensureDesktopDeps().HandleRecordings(w, r)
}

func (s *apiServer) authorizeRecordingSessionAccess(w http.ResponseWriter, r *http.Request, sessionID string) bool {
	return s.ensureDesktopDeps().AuthorizeRecordingSessionAccess(w, r, sessionID)
}

func (s *apiServer) canAccessRecordingMetadata(ctx context.Context, sessionID, recordingActorID string) bool {
	return s.ensureDesktopDeps().CanAccessRecordingMetadata(ctx, sessionID, recordingActorID)
}

func (s *apiServer) stopRecordingBySession(sessionID string) bool {
	return s.ensureDesktopDeps().StopRecordingBySession(sessionID)
}

// Type aliases for types used in cmd/labtether/ test files.
type desktopBridge = desktoppkg.DesktopBridge
type desktopAudioOutbound = desktoppkg.DesktopAudioOutbound
type webrtcSignalingBridge = desktoppkg.WebRTCSignalingBridge
type desktopSessionOptions = desktoppkg.DesktopSessionOptions
type desktopSPICEProxyTarget = desktoppkg.DesktopSPICEProxyTarget
type activeRecording = desktoppkg.ActiveRecording
type clipboardBridge = desktoppkg.ClipboardBridge
type displayBridge = desktoppkg.DisplayBridge
type recordingMetadata = desktoppkg.RecordingMetadata

// Function aliases for exported desktop package functions.
func normalizeDesktopProtocol(raw string) string { return desktoppkg.NormalizeDesktopProtocol(raw) }
func sendDesktopClose(agentConn *agentmgr.AgentConn, sessionID string) {
	desktoppkg.SendDesktopClose(agentConn, sessionID)
}
func sendWebRTCStop(agentConn *agentmgr.AgentConn, sessionID string) {
	desktoppkg.SendWebRTCStop(agentConn, sessionID)
}
func recordingResponsePayload(entry recordingMetadata) map[string]any {
	return desktoppkg.RecordingResponsePayload(entry)
}
func normalizeWebSocketCloseReason(reason string) string {
	return desktoppkg.NormalizeWebSocketCloseReason(reason)
}
func safeWriteClose(wsConn *websocket.Conn, code int, reason string) {
	desktoppkg.SafeWriteClose(wsConn, code, reason)
}

var desktopDebugEnabled = desktoppkg.DesktopDebugEnabled

const maxWebSocketCloseReasonBytes = desktoppkg.MaxWebSocketCloseReasonBytes

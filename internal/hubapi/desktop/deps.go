package desktop

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/policy"
	"github.com/labtether/labtether/internal/protocols"
	"github.com/labtether/labtether/internal/secrets"
	"github.com/labtether/labtether/internal/terminal"
)

// PolicyStateProvider allows the desktop package to read the current policy config.
type PolicyStateProvider interface {
	Current() policy.EvaluatorConfig
}

// Deps holds all dependencies required by the desktop handler package.
// Store interfaces are embedded directly; cross-cutting concerns that live
// in other cmd/labtether subsystems are injected as function fields.
type Deps struct {
	// Store interfaces
	TerminalStore   persistence.TerminalStore
	AssetStore      persistence.AssetStore
	CredentialStore persistence.CredentialStore

	// Database pool for direct queries (recordings).
	DBPool *pgxpool.Pool

	// Agent manager
	AgentMgr *agentmgr.AgentManager

	// Secrets manager for credential decryption.
	SecretsManager *secrets.Manager

	// Policy evaluator state.
	PolicyState PolicyStateProvider

	// Bridge maps (shared with cmd/labtether for WS handler dispatch).
	DesktopBridges           *sync.Map
	WebRTCBridges            *sync.Map
	ClipboardBridges         *sync.Map
	DisplayBridges           *sync.Map
	DesktopDiagnosticWaiters *sync.Map

	// Desktop session options state (protected by mutexes).
	DesktopSessionMu   *sync.RWMutex
	DesktopSessionOpts *map[string]DesktopSessionOptions

	// Desktop SPICE proxy state (protected by mutexes).
	DesktopSPICEMu *sync.RWMutex
	DesktopSPICE   *map[string]DesktopSPICEProxyTarget

	// WebSocket upgrader for desktop streams.
	TerminalWebSocketUpgrader *websocket.Upgrader

	// Constants.
	MaxDesktopInputReadBytes int64
	MaxTargetLength          int

	// Auth middleware injected from cmd/labtether.
	EnforceRateLimit func(w http.ResponseWriter, r *http.Request, bucket string, limit int, window time.Duration) bool
	WrapAuth         func(http.HandlerFunc) http.HandlerFunc

	// Cross-cutting methods injected from cmd/labtether.
	IssueStreamTicket          func(ctx context.Context, sessionID string) (string, time.Time, error)
	DecodeJSONBody             func(w http.ResponseWriter, r *http.Request, dst any) error
	ValidateMaxLen             func(field, value string, maxLen int) error
	PrincipalActorID           func(ctx context.Context) string
	IsOwnerActor               func(actorID string) bool
	CanAccessOwnedSession      func(r *http.Request, sessionActorID string) bool
	GenerateRequestID          func() string
	SanitizeUpstreamError      func(msg string) string
	EnvOrDefaultBool           func(key string, fallback bool) bool
	BrowserStreamTraceID       func(r *http.Request) string
	StreamTraceLogValue        func(traceID string) string
	SanitizeAgentStreamReason  func(raw string) string
	StartBrowserWSKeepalive    func(wsConn *websocket.Conn, writeMu *sync.Mutex, streamLabel string) func()
	TouchBrowserWSReadDeadline func(wsConn *websocket.Conn) error
	UserIDFromContext          func(ctx context.Context) string
	UserRoleFromContext        func(ctx context.Context) string
	CheckSameOrigin            func(r *http.Request) bool

	// Protocol config lookup for manual device connections.
	GetProtocolConfig func(ctx context.Context, assetID, protocol string) (*protocols.ProtocolConfig, error)

	// Proxmox desktop stream routing — injected to avoid circular deps.
	ResolveProxmoxSessionTarget func(assetID string) (any, bool, error)
	HandleProxmoxDesktopStream  func(w http.ResponseWriter, r *http.Request, session terminal.Session, target any)
	HandleDesktopSPICETicket    func(w http.ResponseWriter, r *http.Request, session terminal.Session)
	HandleDesktopSPICEStream    func(w http.ResponseWriter, r *http.Request, session terminal.Session)

	// Recording callbacks — injected to avoid pulling the full recording subsystem.
	StartRecording func(sessionID, assetID, actorID, protocol string) (*ActiveRecording, error)
	StopRecording  func(rec *ActiveRecording)
}

// RegisterRoutes registers all desktop-related HTTP routes on the given handler map.
func RegisterRoutes(handlers map[string]http.HandlerFunc, d *Deps) {
	handlers["/desktop/sessions"] = d.WrapAuth(d.HandleDesktopSessions)
	handlers["/desktop/sessions/"] = d.WrapAuth(d.HandleDesktopSessionActions)
	handlers["/desktop/diagnose/"] = d.WrapAuth(d.HandleDesktopDiagnoseRequest)
	handlers["/recordings"] = d.WrapAuth(d.HandleRecordings)
	handlers["/recordings/"] = d.WrapAuth(d.HandleRecordingActions)
	handlers["/api/v1/nodes/"] = d.WrapAuth(d.HandleNodeSubRoutes)
}

// RegisterWSHandlers registers WebSocket message handlers for desktop-related
// agent messages into the shared router.
func RegisterWSHandlers(router map[string]func(*agentmgr.AgentConn, agentmgr.Message), d *Deps) {
	router[agentmgr.MsgDesktopStarted] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentDesktopStarted(conn, msg)
	}
	router[agentmgr.MsgDesktopData] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentDesktopData(conn, msg)
	}
	router[agentmgr.MsgDesktopClosed] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentDesktopClosed(conn, msg)
	}
	router[agentmgr.MsgDesktopDisplays] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentDesktopDisplays(conn, msg)
	}
	router[agentmgr.MsgDesktopAudioData] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentDesktopAudioData(conn, msg)
	}
	router[agentmgr.MsgDesktopAudioState] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentDesktopAudioState(conn, msg)
	}
	router[agentmgr.MsgWebRTCCapabilities] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentWebRTCCapabilities(conn, msg)
	}
	router[agentmgr.MsgWebRTCStarted] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentWebRTCStarted(conn, msg)
	}
	router[agentmgr.MsgWebRTCAnswer] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentWebRTCAnswer(conn, msg)
	}
	router[agentmgr.MsgWebRTCICE] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentWebRTCICE(conn, msg)
	}
	router[agentmgr.MsgWebRTCStopped] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentWebRTCStopped(conn, msg)
	}
	router[agentmgr.MsgClipboardData] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentClipboardData(conn, msg)
	}
	router[agentmgr.MsgClipboardSetAck] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentClipboardSetAck(conn, msg)
	}
	router[agentmgr.MsgDesktopDiagnosed] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentDesktopDiagnosed(conn, msg)
	}
}

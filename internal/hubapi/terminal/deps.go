package terminal

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/jobqueue"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/policy"
	"github.com/labtether/labtether/internal/protocols"
	"github.com/labtether/labtether/internal/secrets"
	"github.com/labtether/labtether/internal/terminal"
)

// Deps holds all dependencies required by the terminal handler package.
// Store interfaces are embedded directly; cross-cutting concerns that live
// in other cmd/labtether subsystems are injected as function fields.
type Deps struct {
	// Store interfaces
	TerminalStore           persistence.TerminalStore
	TerminalPersistentStore persistence.TerminalPersistentSessionStore
	TerminalBookmarkStore   persistence.TerminalBookmarkStore
	TerminalScrollbackStore persistence.TerminalScrollbackStore
	CredentialStore         persistence.CredentialStore
	AuditStore              persistence.AuditStore
	LogStore                persistence.LogStore
	AssetStore              persistence.AssetStore
	GroupStore              persistence.GroupStore

	// Database pool for direct queries (preferences, snippets, workspace, output buffer).
	DBPool *pgxpool.Pool

	// Agent manager
	AgentMgr *agentmgr.AgentManager

	// Job queue for structured command execution.
	JobQueue *jobqueue.Queue

	// Policy evaluator state.
	PolicyState PolicyStateProvider

	// Secrets manager for credential decryption.
	SecretsManager *secrets.Manager

	// Terminal bridges map (shared with cmd/labtether for WS handler dispatch).
	TerminalBridges *sync.Map

	// Pending agent commands (shared with cmd/labtether).
	PendingAgentCmds *sync.Map

	// ActivePersistentConns tracks the live WebSocket connection for each
	// persistent session (map[persistentSessionID string]*websocket.Conn).
	// Used to evict a previous connection when the same persistent session
	// is attached from a new browser tab or device.
	ActivePersistentConns sync.Map

	// WebSocket upgrader for terminal streams.
	TerminalWebSocketUpgrader websocket.Upgrader

	// Auth middleware injected from cmd/labtether.
	EnforceRateLimit func(w http.ResponseWriter, r *http.Request, bucket string, limit int, window time.Duration) bool
	WrapAuth         func(http.HandlerFunc) http.HandlerFunc

	// Cross-cutting methods injected from cmd/labtether.
	IssueStreamTicket          func(ctx context.Context, sessionID string) (string, time.Time, error)
	ConsumeStreamTicketAuth    func(r *http.Request) (*http.Request, bool)
	PrincipalActorID           func(ctx context.Context) string
	IsOwnerActor               func(actorID string) bool
	DecodeJSONBody             func(w http.ResponseWriter, r *http.Request, dst any) error
	ValidateMaxLen             func(field, value string, maxLen int) error
	AppendAuditEventBestEffort func(event audit.Event, logMessage string)
	AppendLogEventBestEffort   func(event logs.Event, logMessage string)

	// Terminal stream routing — injected to avoid circular deps with proxmox/truenas.
	ResolveDockerExecSessionTarget func(target string) (agentID, containerID string, ok bool)
	HandleDockerExecTerminalStream func(w http.ResponseWriter, r *http.Request, session terminal.Session, agentID, containerID string)
	ResolveProxmoxSessionTarget    func(assetID string) (ProxmoxSessionTarget, bool, error)
	TryProxmoxTerminalStream       func(w http.ResponseWriter, r *http.Request, session terminal.Session, target ProxmoxSessionTarget) error
	ResolveTrueNASSessionTarget    func(assetID string) (TruenasShellTarget, bool, error)
	TryTrueNASTerminalStream       func(w http.ResponseWriter, r *http.Request, session terminal.Session, target TruenasShellTarget) error

	// Protocol config lookup for manual device connections.
	GetProtocolConfig func(ctx context.Context, assetID, protocol string) (*protocols.ProtocolConfig, error)

	// Credential helpers.
	GetAssetTerminalConfig  func(assetID string) (credentials.AssetTerminalConfig, bool, error)
	SaveAssetTerminalConfig func(cfg credentials.AssetTerminalConfig) (credentials.AssetTerminalConfig, error)

	// In-memory terminal store for ring buffer access (scrollback capture).
	TerminalInMemStore *terminal.Store

	// SSH command execution for persistent session cleanup.
	ExecuteSSHCommandFn func(job terminal.CommandJob, mode string, timeout time.Duration, maxOutput int) (string, error)

	// Validation constants.
	MaxActorIDLength int
	MaxTargetLength  int
	MaxCommandLength int
	MaxModeLength    int
}

// PolicyStateProvider allows the terminal package to read the current policy config.
type PolicyStateProvider interface {
	Current() policy.EvaluatorConfig
}

// ProxmoxSessionTarget is a minimal interface to avoid importing the proxmox package.
type ProxmoxSessionTarget = any

// TruenasShellTarget is a minimal interface to avoid importing the truenas package.
type TruenasShellTarget = any

// RegisterRoutes registers all terminal-related HTTP routes on the given handler map.
func RegisterRoutes(handlers map[string]http.HandlerFunc, d *Deps) {
	handlers["/terminal/preferences"] = d.WrapAuth(d.HandleTerminalPreferences)
	handlers["/terminal/snippets"] = d.WrapAuth(d.HandleTerminalSnippets)
	handlers["/terminal/snippets/"] = d.WrapAuth(d.HandleTerminalSnippetActions)
	handlers["/terminal/workspace/tabs"] = d.WrapAuth(d.HandleWorkspaceTabs)
	handlers["/terminal/workspace/tabs/"] = d.WrapAuth(d.HandleWorkspaceTabActions)
	handlers["/terminal/persistent-sessions"] = d.WrapAuth(d.HandlePersistentSessions)
	handlers["/terminal/persistent-sessions/"] = d.WrapAuth(d.HandlePersistentSessionActions)
	handlers["/terminal/bookmarks"] = d.WrapAuth(d.HandleBookmarks)
	handlers["/terminal/bookmarks/"] = d.WrapAuth(d.HandleBookmarkActions)
	handlers["/terminal/sessions"] = d.WrapAuth(d.HandleSessions)
	handlers["/terminal/sessions/"] = d.WrapAuth(d.HandleSessionActions)
	handlers["/terminal/commands/recent"] = d.WrapAuth(d.HandleRecentCommands)
	handlers["/terminal/quick-session"] = d.WrapAuth(d.HandleQuickSession)
}

// RegisterWSHandlers registers WebSocket message handlers for terminal-related
// agent messages into the shared router.
func RegisterWSHandlers(router map[string]func(*agentmgr.AgentConn, agentmgr.Message), d *Deps) {
	router[agentmgr.MsgTerminalProbed] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentTerminalProbed(conn, msg)
	}
	router[agentmgr.MsgTerminalStarted] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentTerminalStarted(conn, msg)
	}
	router[agentmgr.MsgTerminalData] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentTerminalData(conn, msg)
	}
	router[agentmgr.MsgTerminalClosed] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentTerminalClosed(conn, msg)
	}
}

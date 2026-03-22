package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/ssh"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/credentials"
	proxmoxpkg "github.com/labtether/labtether/internal/hubapi/proxmox"
	terminalpkg "github.com/labtether/labtether/internal/hubapi/terminal"
	truenaspkg "github.com/labtether/labtether/internal/hubapi/truenas"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/protocols"
	"github.com/labtether/labtether/internal/terminal"
)

func dbPoolOrNil(db *persistence.PostgresStore) *pgxpool.Pool {
	if db == nil {
		return nil
	}
	return db.Pool()
}

// buildTerminalDeps constructs the terminal.Deps from the apiServer's fields.
// Phase 1 populates all store/config fields. Phase 2 wires SSH helper function
// fields to the Deps methods that now own those implementations, using closures
// that capture d to avoid circular assignment during struct initialization.
func (s *apiServer) buildTerminalDeps() *terminalpkg.Deps {
	d := &terminalpkg.Deps{
		TerminalStore:           s.terminalStore,
		TerminalPersistentStore: s.terminalPersistentStore,
		TerminalBookmarkStore:   s.terminalBookmarkStore,
		TerminalScrollbackStore: s.terminalScrollbackStore,
		CredentialStore:         s.credentialStore,
		AuditStore:              s.auditStore,
		LogStore:                s.logStore,
		AssetStore:              s.assetStore,
		GroupStore:              s.groupStore,

		DBPool: dbPoolOrNil(s.db),

		AgentMgr: s.agentMgr,
		JobQueue: s.jobQueue,

		PolicyState:    s.policyState,
		SecretsManager: s.secretsManager,

		TerminalBridges:  &s.terminalBridges,
		PendingAgentCmds: &s.pendingAgentCmds,

		TerminalWebSocketUpgrader: terminalWebSocketUpgrader,

		EnforceRateLimit: s.enforceRateLimit,
		WrapAuth:         s.withAuth,

		IssueStreamTicket: func(ctx context.Context, sessionID string) (string, time.Time, error) {
			return s.issueStreamTicket(ctx, sessionID)
		},
		ConsumeStreamTicketAuth: func(r *http.Request) (*http.Request, bool) {
			return s.consumeStreamTicketAuth(r)
		},
		PrincipalActorID: func(ctx context.Context) string {
			return principalActorID(ctx)
		},
		IsOwnerActor: func(actorID string) bool {
			return isOwnerActor(actorID)
		},
		DecodeJSONBody: func(w http.ResponseWriter, r *http.Request, dst any) error {
			return decodeJSONBody(w, r, dst)
		},
		ValidateMaxLen: func(field, value string, maxLen int) error {
			return validateMaxLen(field, value, maxLen)
		},
		AppendAuditEventBestEffort: func(event audit.Event, logMessage string) {
			s.appendAuditEventBestEffort(event, logMessage)
		},
		AppendLogEventBestEffort: func(event logs.Event, logMessage string) {
			s.appendLogEventBestEffort(event, logMessage)
		},

		ResolveDockerExecSessionTarget: func(target string) (string, string, bool) {
			return s.resolveDockerExecSessionTarget(target)
		},
		HandleDockerExecTerminalStream: func(w http.ResponseWriter, r *http.Request, session terminal.Session, agentID, containerID string) {
			s.handleDockerExecTerminalStream(w, r, session, agentID, containerID)
		},
		ResolveProxmoxSessionTarget: func(assetID string) (terminalpkg.ProxmoxSessionTarget, bool, error) {
			target, ok, err := s.resolveProxmoxSessionTarget(assetID)
			return target, ok, err
		},
		TryProxmoxTerminalStream: func(w http.ResponseWriter, r *http.Request, session terminal.Session, target terminalpkg.ProxmoxSessionTarget) error {
			return s.tryProxmoxTerminalStream(w, r, session, target.(proxmoxpkg.ProxmoxSessionTarget))
		},
		ResolveTrueNASSessionTarget: func(assetID string) (terminalpkg.TruenasShellTarget, bool, error) {
			target, ok, err := s.resolveTrueNASSessionTarget(assetID)
			return target, ok, err
		},
		TryTrueNASTerminalStream: func(w http.ResponseWriter, r *http.Request, session terminal.Session, target terminalpkg.TruenasShellTarget) error {
			return s.tryTrueNASTerminalStream(w, r, session, target.(truenaspkg.TruenasShellTarget))
		},

		GetProtocolConfig: func(ctx context.Context, assetID, protocol string) (*protocols.ProtocolConfig, error) {
			if s.db == nil {
				return nil, nil
			}
			return s.db.GetProtocolConfig(ctx, assetID, protocol)
		},

		GetAssetTerminalConfig: func(assetID string) (credentials.AssetTerminalConfig, bool, error) {
			if s.credentialStore == nil {
				return credentials.AssetTerminalConfig{}, false, errors.New("credential store not configured")
			}
			return s.credentialStore.GetAssetTerminalConfig(assetID)
		},
		SaveAssetTerminalConfig: func(cfg credentials.AssetTerminalConfig) (credentials.AssetTerminalConfig, error) {
			if s.credentialStore == nil {
				return credentials.AssetTerminalConfig{}, errors.New("credential store not configured")
			}
			return s.credentialStore.SaveAssetTerminalConfig(cfg)
		},

		TerminalInMemStore: s.terminalInMemStore,

		ExecuteSSHCommandFn: func(job terminal.CommandJob, mode string, timeout time.Duration, maxOutput int) (string, error) {
			return executeSSHCommand(job, commandExecutorConfig{
				Mode:           mode,
				Timeout:        timeout,
				MaxOutputBytes: maxOutput,
			})
		},

		MaxActorIDLength: maxActorIDLength,
		MaxTargetLength:  maxTargetLength,
		MaxCommandLength: maxCommandLength,
		MaxModeLength:    maxModeLength,
	}

	return d
}

// ensureTerminalDeps returns the terminal deps, creating and caching on first call.
func (s *apiServer) ensureTerminalDeps() *terminalpkg.Deps {
	if s.terminalDeps != nil {
		return s.terminalDeps
	}
	d := s.buildTerminalDeps()
	s.terminalDeps = d
	return d
}

// Forwarding methods from apiServer to terminal.Deps so that existing
// cmd/labtether/ callers keep compiling without changes.

func (s *apiServer) handleSessions(w http.ResponseWriter, r *http.Request) {
	s.ensureTerminalDeps().HandleSessions(w, r)
}

func (s *apiServer) handleSessionActions(w http.ResponseWriter, r *http.Request) {
	s.ensureTerminalDeps().HandleSessionActions(w, r)
}

func (s *apiServer) handleRecentCommands(w http.ResponseWriter, r *http.Request) {
	s.ensureTerminalDeps().HandleRecentCommands(w, r)
}

func (s *apiServer) handleTerminalPreferences(w http.ResponseWriter, r *http.Request) {
	s.ensureTerminalDeps().HandleTerminalPreferences(w, r)
}

func (s *apiServer) handleTerminalSnippets(w http.ResponseWriter, r *http.Request) {
	s.ensureTerminalDeps().HandleTerminalSnippets(w, r)
}

func (s *apiServer) handleTerminalSnippetActions(w http.ResponseWriter, r *http.Request) {
	s.ensureTerminalDeps().HandleTerminalSnippetActions(w, r)
}

func (s *apiServer) handleWorkspaceTabs(w http.ResponseWriter, r *http.Request) {
	s.ensureTerminalDeps().HandleWorkspaceTabs(w, r)
}

func (s *apiServer) handleWorkspaceTabActions(w http.ResponseWriter, r *http.Request) {
	s.ensureTerminalDeps().HandleWorkspaceTabActions(w, r)
}

func (s *apiServer) handlePersistentSessions(w http.ResponseWriter, r *http.Request) {
	s.ensureTerminalDeps().HandlePersistentSessions(w, r)
}

func (s *apiServer) handlePersistentSessionActions(w http.ResponseWriter, r *http.Request) {
	s.ensureTerminalDeps().HandlePersistentSessionActions(w, r)
}

func (s *apiServer) handleBookmarks(w http.ResponseWriter, r *http.Request) {
	s.ensureTerminalDeps().HandleBookmarks(w, r)
}

func (s *apiServer) handleBookmarkActions(w http.ResponseWriter, r *http.Request) {
	s.ensureTerminalDeps().HandleBookmarkActions(w, r)
}

func (s *apiServer) handleSessionStream(w http.ResponseWriter, r *http.Request, session terminal.Session) {
	s.ensureTerminalDeps().HandleSessionStream(w, r, session)
}

func (s *apiServer) handleSessionStreamTicket(w http.ResponseWriter, r *http.Request, session terminal.Session) {
	s.ensureTerminalDeps().HandleSessionStreamTicket(w, r, session)
}

func (s *apiServer) handleAgentTerminalStream(w http.ResponseWriter, r *http.Request, session terminal.Session) {
	s.ensureTerminalDeps().HandleAgentTerminalStream(w, r, session)
}

func (s *apiServer) handleHubLocalTerminalStream(w http.ResponseWriter, r *http.Request, session terminal.Session) {
	s.ensureTerminalDeps().HandleHubLocalTerminalStream(w, r, session)
}

func (s *apiServer) handleSSHTerminalStream(w http.ResponseWriter, r *http.Request, session terminal.Session) {
	s.ensureTerminalDeps().HandleSSHTerminalStream(w, r, session)
}

func (s *apiServer) processAgentTerminalStarted(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureTerminalDeps().ProcessAgentTerminalStarted(conn, msg)
}

func (s *apiServer) processAgentTerminalData(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureTerminalDeps().ProcessAgentTerminalData(conn, msg)
}

func (s *apiServer) processAgentTerminalClosed(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureTerminalDeps().ProcessAgentTerminalClosed(conn, msg)
}

func (s *apiServer) processAgentTerminalProbed(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureTerminalDeps().ProcessAgentTerminalProbed(conn, msg)
}

func (s *apiServer) closeTerminalBridgesForAsset(assetID string) {
	s.ensureTerminalDeps().CloseTerminalBridgesForAsset(assetID)
}

func (s *apiServer) finalizeAgentTerminalSession(sessionID string, bridgeState *terminalpkg.TerminalBridge, agentConn *agentmgr.AgentConn, startSent bool, closeFn func(*agentmgr.AgentConn, string)) {
	s.ensureTerminalDeps().FinalizeAgentTerminalSession(sessionID, bridgeState, agentConn, startSent, closeFn)
}

func (s *apiServer) probeAgentTmux(agentConn *agentmgr.AgentConn) agentmgr.TerminalProbeResponse {
	return s.ensureTerminalDeps().ProbeAgentTmux(agentConn)
}

func (s *apiServer) startAgentTmuxProbeAsync(agentConn *agentmgr.AgentConn) bool {
	return s.ensureTerminalDeps().StartAgentTmuxProbeAsync(agentConn)
}

func (s *apiServer) markPersistentTerminalStreamDetached(session terminal.Session) {
	s.ensureTerminalDeps().MarkPersistentTerminalStreamDetached(session)
}

func (s *apiServer) bridgeAgentOutput(wsConn *websocket.Conn, outputCh <-chan []byte, closedCh <-chan struct{}, writeMu *sync.Mutex, bridgeState *terminalpkg.TerminalBridge, logContext string) {
	s.ensureTerminalDeps().BridgeAgentOutput(wsConn, outputCh, closedCh, writeMu, bridgeState, logContext)
}

func (s *apiServer) bridgeAgentInput(wsConn *websocket.Conn, agentConn *agentmgr.AgentConn, sessionID string, closedCh <-chan struct{}, bridgeState *terminalpkg.TerminalBridge) (string, error) {
	return s.ensureTerminalDeps().BridgeAgentInput(wsConn, agentConn, sessionID, closedCh, bridgeState)
}

// Type aliases for types used in cmd/labtether/ test files.
type terminalBridge = terminalpkg.TerminalBridge
type terminalStreamEvent = terminalpkg.TerminalStreamEvent
type terminalControlMessage = terminalpkg.TerminalControlMessage

// Standalone helper functions that were defined in the terminal session handlers
// and used by other cmd/labtether files.
func canAccessOwnedSession(r *http.Request, sessionActorID string) bool {
	actorID := principalActorID(r.Context())
	if isOwnerActor(actorID) {
		return true
	}
	return strings.TrimSpace(sessionActorID) == actorID
}

func isOwnerActor(actorID string) bool {
	return strings.TrimSpace(actorID) == "owner"
}

// Function aliases for exported terminal package functions.
func isHubLocalTerminalTarget(target string) bool { return terminalpkg.IsHubLocalTerminalTarget(target) }
func hubLocalTerminalEnabled() bool               { return terminalpkg.HubLocalTerminalEnabled() }

func writeTerminalStatus(wsConn *websocket.Conn, stage, message string, attempt, attempts int, elapsedMs int64) error {
	return terminalpkg.WriteTerminalStatus(wsConn, stage, message, attempt, attempts, elapsedMs)
}
func writeTerminalReady(wsConn *websocket.Conn, message string, elapsedMs int64) error {
	return terminalpkg.WriteTerminalReady(wsConn, message, elapsedMs)
}
func writeTerminalError(wsConn *websocket.Conn, stage, message string) error {
	return terminalpkg.WriteTerminalError(wsConn, stage, message)
}
func isControlMessage(payload []byte) bool { return terminalpkg.IsControlMessage(payload) }
func parseTerminalSize(colsRaw, rowsRaw string) (int, int) {
	return terminalpkg.ParseTerminalSize(colsRaw, rowsRaw)
}
func sanitizeAgentStreamReason(raw string) string { return terminalpkg.SanitizeAgentStreamReason(raw) }
func sendTerminalClose(agentConn *agentmgr.AgentConn, sessionID string) {
	terminalpkg.SendTerminalClose(agentConn, sessionID)
}

func dialSSHWithRetry(addr string, baseConfig *ssh.ClientConfig, attemptTimeout time.Duration, maxAttempts int, retryDelay time.Duration, dialFn func(string, string, *ssh.ClientConfig) (*ssh.Client, error), sleepFn func(time.Duration), onAttempt func(int, int)) (*ssh.Client, int, error) {
	return terminalpkg.DialSSHWithRetry(addr, baseConfig, attemptTimeout, maxAttempts, retryDelay, dialFn, sleepFn, onAttempt)
}

func startSSHInteractiveSessionWithTimeout(sshClient *ssh.Client, cols, rows int, startCommand string, timeout time.Duration) (*ssh.Session, io.WriteCloser, io.Reader, io.Reader, error) {
	return terminalpkg.StartSSHInteractiveSessionWithTimeout(sshClient, cols, rows, startCommand, timeout)
}

func handleTerminalControlMessage(sshSession *ssh.Session, stdin io.Writer, payload []byte) (bool, error) {
	return terminalpkg.HandleTerminalControlMessage(sshSession, stdin, payload)
}

func handleLocalTerminalControlMessage(ptmx *os.File, payload []byte) (bool, error) {
	return terminalpkg.HandleLocalTerminalControlMessage(ptmx, payload)
}

func newTerminalOutputRecorder(pool *pgxpool.Pool, sessionID string) *terminalpkg.TerminalOutputRecorder {
	return terminalpkg.NewTerminalOutputRecorder(pool, sessionID)
}

func replayBuffer(ctx context.Context, pool *pgxpool.Pool, sessionID string) ([][]byte, error) {
	return terminalpkg.ReplayBuffer(ctx, pool, sessionID)
}

func cleanupExpiredBuffers(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	return terminalpkg.CleanupExpiredBuffers(ctx, pool)
}

func persistentTmuxCleanupCommand(tmuxSessionName string) string {
	return terminalpkg.PersistentTmuxCleanupCommand(tmuxSessionName)
}

// Constants aliases.
const (
	hubLocalTerminalTarget    = terminalpkg.HubLocalTerminalTarget
	envEnableHubLocalTerminal = terminalpkg.EnvEnableHubLocalTerminal
	sshDialAttemptTimeout     = terminalpkg.SSHDialAttemptTimeout
	sshDialMaxAttempts        = terminalpkg.SSHDialMaxAttempts
	sshDialRetryDelay         = terminalpkg.SSHDialRetryDelay
	sshShellStartupTimeout    = terminalpkg.SSHShellStartupTimeout
	terminalStreamWriteDeadline = terminalpkg.TerminalStreamWriteDeadline
)

var errPersistentTerminalCleanupUnavailable = terminalpkg.ErrPersistentTerminalCleanupUnavailable

// Test-seam: cmd/labtether tests write to terminalpkg.PersistentTmuxCleanupSSHFunc directly.
var persistentTmuxCleanupAgentFunc = terminalpkg.PersistentTmuxCleanupAgentFunc
var persistentTmuxCleanupSSHFunc = terminalpkg.PersistentTmuxCleanupSSHFunc

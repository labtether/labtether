package dockerpkg

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/connectors/docker"
	"github.com/labtether/labtether/internal/connectorsdk"
)

// Deps holds all dependencies required by the docker handler package.
// Store interfaces are embedded directly; cross-cutting concerns that live
// in other cmd/labtether subsystems are injected as function fields.
type Deps struct {
	// Docker coordinator
	DockerCoordinator *docker.Coordinator

	// Agent manager
	AgentMgr *agentmgr.AgentManager

	// Docker exec bridges (shared with cmd/labtether for WS handler dispatch).
	DockerExecBridges *sync.Map

	// WebSocket upgrader for exec streams.
	TerminalWebSocketUpgrader websocket.Upgrader

	// Auth middleware injected from cmd/labtether.
	WrapAuth func(http.HandlerFunc) http.HandlerFunc

	// Cross-cutting methods injected from cmd/labtether.
	DecodeJSONBody             func(w http.ResponseWriter, r *http.Request, dst any) error
	SanitizeUpstreamError      func(msg string) string
	ParseTerminalSize          func(colsRaw, rowsRaw string) (int, int)
	IsControlMessage           func(payload []byte) bool
	StartBrowserWSKeepalive    func(wsConn *websocket.Conn, writeMu *sync.Mutex, streamLabel string) func()
	TouchBrowserWSReadDeadline func(wsConn *websocket.Conn) error

	// Broadcaster callback for event dispatch.
	Broadcast func(eventType string, data json.RawMessage)

	// Docker coordinator action execution (wrapping dockerCoordinator.ExecuteAction).
	ExecuteDockerAction func(ctx context.Context, actionID string, req connectorsdk.ActionRequest) (connectorsdk.ActionResult, error)

	// Docker collector trigger on discovery.
	TriggerDockerCollectorRunForDiscovery func()
}

// DockerExecBridge is the bridge state for a docker exec session.
type DockerExecBridge struct {
	OutputCh        chan []byte
	ClosedCh        chan struct{}
	ExpectedAgentID string
	reasonMu        sync.Mutex
	reason          string
	closeMu         sync.Once
}

// Close closes the bridge with the given reason (first reason wins).
func (b *DockerExecBridge) Close(reason string) {
	if b == nil {
		return
	}
	trimmed := strings.TrimSpace(reason)
	if trimmed != "" {
		b.reasonMu.Lock()
		if b.reason == "" {
			b.reason = trimmed
		}
		b.reasonMu.Unlock()
	}
	b.closeMu.Do(func() {
		close(b.ClosedCh)
	})
}

// ReasonOr returns the close reason, or the fallback if no reason was set.
func (b *DockerExecBridge) ReasonOr(fallback string) string {
	if b == nil {
		return fallback
	}
	b.reasonMu.Lock()
	defer b.reasonMu.Unlock()
	if strings.TrimSpace(b.reason) != "" {
		return b.reason
	}
	return fallback
}

// TrySendOutput attempts to send output data to the bridge channel.
func (b *DockerExecBridge) TrySendOutput(payload []byte) {
	if b == nil {
		return
	}
	select {
	case <-b.ClosedCh:
		return
	default:
	}
	defer func() { _ = recover() }()
	select {
	case b.OutputCh <- payload:
	default:
	}
}

// MatchesAgent returns true if the bridge is associated with the given asset ID.
func (b *DockerExecBridge) MatchesAgent(assetID string) bool {
	if b == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(b.ExpectedAgentID), strings.TrimSpace(assetID))
}

// ControlMessage represents a terminal control message for exec streams.
type ControlMessage struct {
	Type string `json:"type"`
	Cols int    `json:"cols,omitempty"`
	Rows int    `json:"rows,omitempty"`
	Data string `json:"data,omitempty"`
}

// RegisterRoutes registers all docker-related HTTP routes on the given handler map.
func RegisterRoutes(handlers map[string]http.HandlerFunc, d *Deps) {
	handlers["/api/v1/docker/hosts"] = d.WrapAuth(d.HandleDockerHosts)
	handlers["/api/v1/docker/hosts/"] = d.WrapAuth(d.HandleDockerHostActions)
	handlers["/api/v1/docker/containers/"] = d.WrapAuth(d.HandleDockerContainerActions)
	handlers["/api/v1/docker/stacks/"] = d.WrapAuth(d.HandleDockerStackActions)
}

// RegisterWSHandlers registers WebSocket message handlers for docker-related
// agent messages into the shared router.
func RegisterWSHandlers(router map[string]func(*agentmgr.AgentConn, agentmgr.Message), d *Deps) {
	router[agentmgr.MsgDockerDiscovery] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentDockerDiscovery(conn, msg)
	}
	router[agentmgr.MsgDockerDiscoveryDelta] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentDockerDiscoveryDelta(conn, msg)
	}
	router[agentmgr.MsgDockerStats] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentDockerStats(conn, msg)
	}
	router[agentmgr.MsgDockerEvents] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentDockerEvents(conn, msg)
	}
	router[agentmgr.MsgDockerActionResult] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentDockerActionResult(conn, msg)
	}
	router[agentmgr.MsgDockerExecStarted] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentDockerExecStartedMessage(conn, msg)
	}
	router[agentmgr.MsgDockerExecData] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentDockerExecDataMessage(conn, msg)
	}
	router[agentmgr.MsgDockerExecClosed] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentDockerExecClosedMessage(conn, msg)
	}
	router[agentmgr.MsgDockerLogsStream] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentDockerLogsStreamMessage(conn, msg)
	}
	router[agentmgr.MsgDockerComposeResult] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		d.ProcessAgentDockerComposeResult(conn, msg)
	}
}

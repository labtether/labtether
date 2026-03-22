package main

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/connectorsdk"
	dockerpkg "github.com/labtether/labtether/internal/hubapi/dockerpkg"
	"github.com/labtether/labtether/internal/terminal"
)

// buildDockerDeps constructs the dockerpkg.Deps from the apiServer's fields.
func (s *apiServer) buildDockerDeps() *dockerpkg.Deps {
	return &dockerpkg.Deps{
		DockerCoordinator: s.dockerCoordinator,
		AgentMgr:          s.agentMgr,
		DockerExecBridges: &s.dockerExecBridges,

		TerminalWebSocketUpgrader: terminalWebSocketUpgrader,

		WrapAuth: s.withAuth,

		DecodeJSONBody: func(w http.ResponseWriter, r *http.Request, dst any) error {
			return decodeJSONBody(w, r, dst)
		},
		SanitizeUpstreamError: func(msg string) string {
			return sanitizeUpstreamError(msg)
		},
		ParseTerminalSize: func(colsRaw, rowsRaw string) (int, int) {
			return parseTerminalSize(colsRaw, rowsRaw)
		},
		IsControlMessage: func(payload []byte) bool {
			return isControlMessage(payload)
		},
		StartBrowserWSKeepalive: func(wsConn *websocket.Conn, writeMu *sync.Mutex, streamLabel string) func() {
			return startBrowserWebSocketKeepalive(wsConn, writeMu, streamLabel)
		},
		TouchBrowserWSReadDeadline: func(wsConn *websocket.Conn) error {
			return touchBrowserWebSocketReadDeadline(wsConn)
		},
		Broadcast: func(eventType string, data json.RawMessage) {
			if s.broadcaster != nil {
				s.broadcaster.Broadcast(eventType, data)
			}
		},
		ExecuteDockerAction: func(ctx context.Context, actionID string, req connectorsdk.ActionRequest) (connectorsdk.ActionResult, error) {
			if s.dockerCoordinator == nil {
				return connectorsdk.ActionResult{}, nil
			}
			return s.dockerCoordinator.ExecuteAction(ctx, actionID, req)
		},
		TriggerDockerCollectorRunForDiscovery: func() {
			s.triggerDockerCollectorRunForDiscovery()
		},
	}
}

// ensureDockerDeps returns the docker deps, creating and caching on first call.
func (s *apiServer) ensureDockerDeps() *dockerpkg.Deps {
	if s.dockerDeps != nil {
		return s.dockerDeps
	}
	d := s.buildDockerDeps()
	s.dockerDeps = d
	return d
}

// Forwarding methods from apiServer to dockerpkg.Deps so that existing
// cmd/labtether/ callers keep compiling without changes.

func (s *apiServer) handleDockerHosts(w http.ResponseWriter, r *http.Request) {
	s.ensureDockerDeps().HandleDockerHosts(w, r)
}

func (s *apiServer) handleDockerHostActions(w http.ResponseWriter, r *http.Request) {
	s.ensureDockerDeps().HandleDockerHostActions(w, r)
}

func (s *apiServer) handleDockerContainerActions(w http.ResponseWriter, r *http.Request) {
	s.ensureDockerDeps().HandleDockerContainerActions(w, r)
}

func (s *apiServer) handleDockerStackActions(w http.ResponseWriter, r *http.Request) {
	s.ensureDockerDeps().HandleDockerStackActions(w, r)
}

func (s *apiServer) resolveDockerExecSessionTarget(target string) (agentID, containerID string, ok bool) {
	return s.ensureDockerDeps().ResolveDockerExecSessionTarget(target)
}

func (s *apiServer) handleDockerExecTerminalStream(w http.ResponseWriter, r *http.Request, session terminal.Session, agentID, containerID string) {
	s.ensureDockerDeps().HandleDockerExecTerminalStream(w, r, session, agentID, containerID)
}

func (s *apiServer) processAgentDockerDiscovery(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureDockerDeps().ProcessAgentDockerDiscovery(conn, msg)
}

func (s *apiServer) processAgentDockerDiscoveryDelta(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureDockerDeps().ProcessAgentDockerDiscoveryDelta(conn, msg)
}

func (s *apiServer) processAgentDockerStats(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureDockerDeps().ProcessAgentDockerStats(conn, msg)
}

func (s *apiServer) processAgentDockerEvents(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureDockerDeps().ProcessAgentDockerEvents(conn, msg)
}

func (s *apiServer) processAgentDockerActionResult(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureDockerDeps().ProcessAgentDockerActionResult(conn, msg)
}

func (s *apiServer) processAgentDockerExecStartedMessage(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureDockerDeps().ProcessAgentDockerExecStartedMessage(conn, msg)
}

func (s *apiServer) processAgentDockerExecDataMessage(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureDockerDeps().ProcessAgentDockerExecDataMessage(conn, msg)
}

func (s *apiServer) processAgentDockerExecClosedMessage(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureDockerDeps().ProcessAgentDockerExecClosedMessage(conn, msg)
}

func (s *apiServer) processAgentDockerLogsStreamMessage(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureDockerDeps().ProcessAgentDockerLogsStreamMessage(conn, msg)
}

func (s *apiServer) processAgentDockerComposeResult(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureDockerDeps().ProcessAgentDockerComposeResult(conn, msg)
}

func (s *apiServer) processAgentDockerExecStarted(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureDockerDeps().ProcessAgentDockerExecStarted(conn, msg)
}

func (s *apiServer) processAgentDockerExecData(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureDockerDeps().ProcessAgentDockerExecData(conn, msg)
}

func (s *apiServer) processAgentDockerExecClosed(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	s.ensureDockerDeps().ProcessAgentDockerExecClosed(conn, msg)
}

func (s *apiServer) closeDockerExecBridgesForAsset(assetID string) {
	s.ensureDockerDeps().CloseDockerExecBridgesForAsset(assetID)
}

// Type aliases for types used in cmd/labtether/ test files.
type dockerExecBridge = dockerpkg.DockerExecBridge

// Function aliases for exported dockerpkg functions.
func normalizeDockerHostLookupID(value string) string {
	return dockerpkg.NormalizeDockerHostLookupID(value)
}
func dockerExecCommandFromQuery(raw string) []string {
	return dockerpkg.DockerExecCommandFromQuery(raw)
}

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/connectorsdk"
	dockerpkg "github.com/labtether/labtether/internal/hubapi/dockerpkg"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/terminal"
)

// buildDockerDeps constructs the dockerpkg.Deps from the apiServer's fields.
func (s *apiServer) buildDockerDeps() *dockerpkg.Deps {
	return &dockerpkg.Deps{
		DockerCoordinator: s.dockerCoordinator,
		AgentMgr:          s.agentMgr,
		AssetStore:        s.assetStore,
		DockerExecBridges: &s.dockerExecBridges,
		ClaimPersistentTerminalStream: func(session terminal.Session, conn *websocket.Conn) (func(), func()) {
			return s.ensureTerminalDeps().ClaimPersistentTerminalStream(session, conn)
		},

		TerminalWebSocketUpgrader: terminalWebSocketUpgrader,

		WrapAuth: s.withAuth,

		DecodeJSONBody: func(w http.ResponseWriter, r *http.Request, dst any) error {
			return shared.DecodeJSONBody(w, r, dst)
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
			return shared.StartBrowserWebSocketKeepalive(wsConn, writeMu, streamLabel)
		},
		TouchBrowserWSReadDeadline: func(wsConn *websocket.Conn) error {
			return shared.TouchBrowserWebSocketReadDeadline(wsConn)
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
		EvaluateAssetGuardrails: s.ensureGroupFeaturesDeps().EvaluateAssetGuardrails,
		TriggerDockerCollectorRunForDiscovery: func() {
			s.triggerDockerCollectorRunForDiscovery()
		},
	}
}

// ensureDockerDeps returns the docker deps, creating and caching on first call.
func (s *apiServer) ensureDockerDeps() *dockerpkg.Deps {
	s.dockerDepsOnce.Do(func() {
		s.dockerDeps = s.buildDockerDeps()
	})
	return s.dockerDeps
}

// The three handlers below stay in cmd: each refuses a Docker write while
// Settings › Integration actions is off (refuseIntegrationActionWhenOff, cmd
// code) before the package handler runs. The v1 route and its v2 wrapper both
// call them, so the check is in one place. Other callers call
// ensureDockerDeps() themselves, when the request or call is made.

func (s *apiServer) handleDockerHostActions(w http.ResponseWriter, r *http.Request) {
	if s.refuseIntegrationActionWhenOff(w, r) {
		return
	}
	s.ensureDockerDeps().HandleDockerHostActions(w, r)
}

func (s *apiServer) handleDockerContainerActions(w http.ResponseWriter, r *http.Request) {
	if s.refuseIntegrationActionWhenOff(w, r) {
		return
	}
	s.ensureDockerDeps().HandleDockerContainerActions(w, r)
}

func (s *apiServer) handleDockerStackActions(w http.ResponseWriter, r *http.Request) {
	if s.refuseIntegrationActionWhenOff(w, r) {
		return
	}
	s.ensureDockerDeps().HandleDockerStackActions(w, r)
}

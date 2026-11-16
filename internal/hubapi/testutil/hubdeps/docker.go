package hubdeps

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/hubapi/collectors"
	"github.com/labtether/labtether/internal/hubapi/dockerpkg"
	"github.com/labtether/labtether/internal/hubapi/shared"
	terminalpkg "github.com/labtether/labtether/internal/hubapi/terminal"
	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/terminal"
)

// BuildDockerDeps mirrors cmd's buildDockerDeps field by field. Stand-ins
// replace only cmd's route auth, the WebSocket origin check and cmd's Docker
// auto-collector trigger. Like cmd's test server, the Hub has no event
// broadcaster, so events are dropped as cmd drops them when it has none.
func (h *Hub) BuildDockerDeps() *dockerpkg.Deps {
	return &dockerpkg.Deps{
		DockerCoordinator: h.DockerCoordinator,
		AgentMgr:          h.AgentMgr,
		AssetStore:        h.AssetStore,
		DockerExecBridges: &h.DockerExecBridges,
		ClaimPersistentTerminalStream: func(session terminal.Session, conn *websocket.Conn) (func(), func()) {
			return h.TerminalDeps().ClaimPersistentTerminalStream(session, conn)
		},

		TerminalWebSocketUpgrader: *testutil.NewStreamUpgrader(),

		WrapAuth: testutil.RouteAuthNotTested(h.t),

		DecodeJSONBody:             shared.DecodeJSONBody,
		SanitizeUpstreamError:      collectors.SanitizeUpstreamError,
		ParseTerminalSize:          terminalpkg.ParseTerminalSize,
		IsControlMessage:           terminalpkg.IsControlMessage,
		StartBrowserWSKeepalive:    shared.StartBrowserWebSocketKeepalive,
		TouchBrowserWSReadDeadline: shared.TouchBrowserWebSocketReadDeadline,
		Broadcast:                  func(string, json.RawMessage) {},
		ExecuteDockerAction: func(ctx context.Context, actionID string, req connectorsdk.ActionRequest) (connectorsdk.ActionResult, error) {
			if h.DockerCoordinator == nil {
				return connectorsdk.ActionResult{}, nil
			}
			return h.DockerCoordinator.ExecuteAction(ctx, actionID, req)
		},
		EvaluateAssetGuardrails:               h.GroupFeaturesDeps().EvaluateAssetGuardrails,
		TriggerDockerCollectorRunForDiscovery: dockerCollectorRunNotTested(h.t),
	}
}

// DockerDeps mirrors cmd's ensureDockerDeps: built on first use and kept,
// so the exec stream owners it holds last for the test.
func (h *Hub) DockerDeps() *dockerpkg.Deps {
	h.dockerOnce.Do(func() { h.dockerDeps = h.BuildDockerDeps() })
	return h.dockerDeps
}

// dockerCollectorRunNotTested stands in for cmd's
// triggerDockerCollectorRunForDiscovery, which starts a run of each Docker
// collector in the Hub's collector store. It fails the test if reached;
// that code is cmd's and is tested in cmd/labtether.
func dockerCollectorRunNotTested(t *testing.T) func() {
	return func() {
		t.Errorf("Docker discovery reached the collector run stand-in; that trigger is tested in cmd/labtether")
	}
}

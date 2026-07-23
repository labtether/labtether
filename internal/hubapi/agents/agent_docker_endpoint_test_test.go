package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/hubapi/testutil"
)

type dockerEndpointTestOutcome struct {
	result agentmgr.DockerEndpointTestResultData
	err    error
}

func newDockerEndpointTestAgentConn(t *testing.T, assetID, platform string) (*agentmgr.AgentManager, *agentmgr.AgentConn, *websocket.Conn) {
	t.Helper()
	serverConnCh := make(chan *websocket.Conn, 1)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		serverConnCh <- conn
		<-release
		_ = conn.Close()
	}))
	peer, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		server.Close()
		t.Fatalf("dial test agent WebSocket: %v", err)
	}
	serverConn := <-serverConnCh
	conn := agentmgr.NewAgentConn(serverConn, assetID, platform)
	manager := agentmgr.NewManager()
	manager.Register(conn)
	t.Cleanup(func() {
		manager.Unregister(assetID)
		_ = peer.Close()
		close(release)
		server.Close()
	})
	return manager, conn, peer
}

func readDockerEndpointTestRequest(t *testing.T, peer *websocket.Conn) (agentmgr.Message, agentmgr.DockerEndpointTestData) {
	t.Helper()
	if err := peer.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	var msg agentmgr.Message
	if err := peer.ReadJSON(&msg); err != nil {
		t.Fatalf("read Docker endpoint test request: %v", err)
	}
	if msg.Type != agentmgr.MsgDockerEndpointTest {
		t.Fatalf("message type = %q, want %q", msg.Type, agentmgr.MsgDockerEndpointTest)
	}
	var request agentmgr.DockerEndpointTestData
	if err := json.Unmarshal(msg.Data, &request); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	if err := request.Validate(); err != nil {
		t.Fatalf("Hub sent invalid request: %v", err)
	}
	if msg.ID != request.RequestID {
		t.Fatalf("envelope id = %q, request id = %q", msg.ID, request.RequestID)
	}
	return msg, request
}

func dockerEndpointResultMessage(t *testing.T, envelopeID string, result agentmgr.DockerEndpointTestResultData) agentmgr.Message {
	t.Helper()
	if err := result.Validate(); err != nil {
		t.Fatalf("test result is invalid: %v", err)
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	return agentmgr.Message{Type: agentmgr.MsgDockerEndpointTestResult, ID: envelopeID, Data: data}
}

func TestRunDockerEndpointTestRequiresExactAgentEnvelopeAssetAndEndpoint(t *testing.T) {
	manager, conn, peer := newDockerEndpointTestAgentConn(t, "node-1", "linux")
	deps := &Deps{AgentMgr: manager, DockerEndpointTestTimeout: time.Second}
	done := make(chan dockerEndpointTestOutcome, 1)
	go func() {
		result, err := deps.RunDockerEndpointTest(context.Background(), "node-1", "unix:///run/docker.sock")
		done <- dockerEndpointTestOutcome{result: result, err: err}
	}()

	msg, request := readDockerEndpointTestRequest(t, peer)
	valid := agentmgr.DockerEndpointTestResultData{
		RequestID: request.RequestID,
		AssetID:   request.AssetID,
		Endpoint:  request.Endpoint,
		Status:    agentmgr.DockerEndpointTestStatusReachable,
		Message:   "Docker endpoint is reachable",
	}

	deps.ProcessAgentDockerEndpointTestResult(&agentmgr.AgentConn{AssetID: "node-1"}, dockerEndpointResultMessage(t, msg.ID, valid))
	assertDockerEndpointTestStillPending(t, deps, request.RequestID, done)

	deps.ProcessAgentDockerEndpointTestResult(&agentmgr.AgentConn{AssetID: "node-2"}, dockerEndpointResultMessage(t, msg.ID, valid))
	assertDockerEndpointTestStillPending(t, deps, request.RequestID, done)

	deps.ProcessAgentDockerEndpointTestResult(conn, dockerEndpointResultMessage(t, "wrong-envelope", valid))
	assertDockerEndpointTestStillPending(t, deps, request.RequestID, done)

	wrongAsset := valid
	wrongAsset.AssetID = "node-2"
	deps.ProcessAgentDockerEndpointTestResult(conn, dockerEndpointResultMessage(t, msg.ID, wrongAsset))
	assertDockerEndpointTestStillPending(t, deps, request.RequestID, done)

	wrongEndpoint := valid
	wrongEndpoint.Endpoint = "unix:///run/other.sock"
	deps.ProcessAgentDockerEndpointTestResult(conn, dockerEndpointResultMessage(t, msg.ID, wrongEndpoint))
	assertDockerEndpointTestStillPending(t, deps, request.RequestID, done)

	deps.ProcessAgentDockerEndpointTestResult(conn, dockerEndpointResultMessage(t, msg.ID, valid))
	select {
	case got := <-done:
		if got.err != nil || got.result != valid {
			t.Fatalf("outcome = %+v, want exact valid result", got)
		}
	case <-time.After(time.Second):
		t.Fatal("correct result did not complete pending endpoint test")
	}
}

func assertDockerEndpointTestStillPending(
	t *testing.T,
	deps *Deps,
	requestID string,
	done <-chan dockerEndpointTestOutcome,
) {
	t.Helper()
	if _, ok := deps.DockerEndpointTestBridges.Load(requestID); !ok {
		t.Fatal("wrong result removed pending endpoint test")
	}
	select {
	case got := <-done:
		t.Fatalf("wrong result completed endpoint test: %+v", got)
	default:
	}
}

func TestRunDockerEndpointTestTimesOutBoundedly(t *testing.T) {
	manager, _, peer := newDockerEndpointTestAgentConn(t, "node-1", "linux")
	deps := &Deps{AgentMgr: manager, DockerEndpointTestTimeout: 20 * time.Millisecond}
	done := make(chan error, 1)
	go func() {
		_, err := deps.RunDockerEndpointTest(context.Background(), "node-1", "/run/docker.sock")
		done <- err
	}()
	_, request := readDockerEndpointTestRequest(t, peer)
	select {
	case err := <-done:
		if !errors.Is(err, ErrDockerEndpointTestTimeout) {
			t.Fatalf("timeout error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("endpoint test did not honor bounded timeout")
	}
	if _, ok := deps.DockerEndpointTestBridges.Load(request.RequestID); ok {
		t.Fatal("timed-out endpoint test left a pending bridge")
	}
}

func TestHandleAgentSettingsDockerTestUsesTypedProbeAndMapsClosedResult(t *testing.T) {
	tests := []struct {
		name       string
		status     agentmgr.DockerEndpointTestStatus
		code       agentmgr.DockerEndpointTestCode
		message    string
		wantStatus int
		wantOK     string
	}{
		{name: "reachable", status: agentmgr.DockerEndpointTestStatusReachable, message: "Docker endpoint is reachable", wantStatus: http.StatusOK, wantOK: `"ok":true`},
		{name: "unreachable", status: agentmgr.DockerEndpointTestStatusFailed, code: agentmgr.DockerEndpointTestCodeUnreachable, message: "Docker endpoint is unreachable", wantStatus: http.StatusBadRequest, wantOK: `"ok":false`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, conn, peer := newDockerEndpointTestAgentConn(t, "node-1", "windows")
			deps := &Deps{
				AgentMgr:                  manager,
				RuntimeStore:              testutil.NewRuntimeSettingsStore(),
				SecretsManager:            testutil.TestSecretsManager(t),
				EnforceRateLimit:          testutil.NoopRateLimit,
				DockerEndpointTestTimeout: time.Second,
			}
			req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/node-1/settings/test-docker", bytes.NewBufferString(`{"enabled":"true","endpoint":"npipe:////./pipe/docker_engine"}`))
			rec := httptest.NewRecorder()
			done := make(chan struct{})
			go func() {
				deps.HandleAgentSettingsDockerTest(rec, req, "node-1")
				close(done)
			}()
			msg, request := readDockerEndpointTestRequest(t, peer)
			if request.Endpoint != "npipe:////./pipe/docker_engine" {
				t.Fatalf("typed request endpoint = %q, want canonical npipe endpoint", request.Endpoint)
			}
			result := agentmgr.DockerEndpointTestResultData{
				RequestID: request.RequestID,
				AssetID:   request.AssetID,
				Endpoint:  request.Endpoint,
				Status:    tt.status,
				Code:      tt.code,
				Message:   tt.message,
			}
			deps.ProcessAgentDockerEndpointTestResult(conn, dockerEndpointResultMessage(t, msg.ID, result))
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("HTTP handler did not complete")
			}
			if rec.Code != tt.wantStatus {
				t.Fatalf("HTTP status = %d, want %d: %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tt.wantOK) || !strings.Contains(rec.Body.String(), tt.message) {
				t.Fatalf("unexpected response body: %s", rec.Body.String())
			}
		})
	}
}

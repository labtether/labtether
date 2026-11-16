package resources_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/hubapi/testutil/hubdeps"
)

func TestHandleNetworkActionBridgesAgentResult(t *testing.T) {
	sut := hubdeps.New(t)
	sut.AgentMgr = agentmgr.NewManager()

	serverConn, clientConn, cleanup := testutil.CreateWSPairForNetworkTest(t)
	defer cleanup()

	sut.AgentMgr.Register(agentmgr.NewAgentConn(serverConn, "node-1", "linux"))
	defer sut.AgentMgr.Unregister("node-1")

	done := make(chan struct{})
	go func() {
		defer close(done)
		var outbound agentmgr.Message
		if err := clientConn.ReadJSON(&outbound); err != nil {
			t.Errorf("read outbound: %v", err)
			return
		}
		if outbound.Type != agentmgr.MsgNetworkAction {
			t.Errorf("outbound type=%q, want %q", outbound.Type, agentmgr.MsgNetworkAction)
			return
		}

		var req agentmgr.NetworkActionData
		if err := json.Unmarshal(outbound.Data, &req); err != nil {
			t.Errorf("decode network action payload: %v", err)
			return
		}
		if req.Action != "apply" {
			t.Errorf("action=%q, want apply", req.Action)
		}
		if req.Method != "netplan" {
			t.Errorf("method=%q, want netplan", req.Method)
		}

		raw, _ := json.Marshal(agentmgr.NetworkResultData{
			RequestID: req.RequestID,
			OK:        true,
			Output:    "applied",
		})
		sut.ResourcesDeps().ProcessAgentNetworkResult(&agentmgr.AgentConn{AssetID: "node-1"}, agentmgr.Message{
			Type: agentmgr.MsgNetworkResult,
			ID:   req.RequestID,
			Data: raw,
		})
	}()

	req := httptest.NewRequest(http.MethodPost, "/network/node-1/apply", strings.NewReader(`{"method":"netplan"}`))
	rec := httptest.NewRecorder()
	sut.ResourcesDeps().HandleNetworks(rec, req)

	<-done

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	var response agentmgr.NetworkResultData
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !response.OK {
		t.Fatalf("expected ok=true, got %+v", response)
	}
}

func TestHandleNetworkActionPropagatesAgentError(t *testing.T) {
	sut := hubdeps.New(t)
	sut.AgentMgr = agentmgr.NewManager()

	serverConn, clientConn, cleanup := testutil.CreateWSPairForNetworkTest(t)
	defer cleanup()

	sut.AgentMgr.Register(agentmgr.NewAgentConn(serverConn, "node-2", "linux"))
	defer sut.AgentMgr.Unregister("node-2")

	done := make(chan struct{})
	go func() {
		defer close(done)
		var outbound agentmgr.Message
		if err := clientConn.ReadJSON(&outbound); err != nil {
			t.Errorf("read outbound: %v", err)
			return
		}

		var req agentmgr.NetworkActionData
		if err := json.Unmarshal(outbound.Data, &req); err != nil {
			t.Errorf("decode network action payload: %v", err)
			return
		}

		raw, _ := json.Marshal(agentmgr.NetworkResultData{
			RequestID: req.RequestID,
			OK:        false,
			Error:     "invalid method",
		})
		sut.ResourcesDeps().ProcessAgentNetworkResult(&agentmgr.AgentConn{AssetID: "node-2"}, agentmgr.Message{
			Type: agentmgr.MsgNetworkResult,
			ID:   req.RequestID,
			Data: raw,
		})
	}()

	req := httptest.NewRequest(http.MethodPost, "/network/node-2/apply", strings.NewReader(`{"method":"bad"}`))
	rec := httptest.NewRecorder()
	sut.ResourcesDeps().HandleNetworks(rec, req)

	<-done

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid method") {
		t.Fatalf("expected invalid method error, got %s", rec.Body.String())
	}
}

func TestHandleNetworkListBridgesAgentInventory(t *testing.T) {
	sut := hubdeps.New(t)
	sut.AgentMgr = agentmgr.NewManager()

	serverConn, clientConn, cleanup := testutil.CreateWSPairForNetworkTest(t)
	defer cleanup()

	sut.AgentMgr.Register(agentmgr.NewAgentConn(serverConn, "node-3", "linux"))
	defer sut.AgentMgr.Unregister("node-3")

	done := make(chan struct{})
	go func() {
		defer close(done)

		var outbound agentmgr.Message
		if err := clientConn.ReadJSON(&outbound); err != nil {
			t.Errorf("read outbound: %v", err)
			return
		}
		if outbound.Type != agentmgr.MsgNetworkList {
			t.Errorf("outbound type=%q, want %q", outbound.Type, agentmgr.MsgNetworkList)
			return
		}

		var req agentmgr.NetworkListData
		if err := json.Unmarshal(outbound.Data, &req); err != nil {
			t.Errorf("decode network list payload: %v", err)
			return
		}

		raw, _ := json.Marshal(agentmgr.NetworkListedData{
			RequestID: req.RequestID,
			Interfaces: []agentmgr.NetInterface{{
				Name:  "eth0",
				State: "up",
				IPs:   []string{"192.168.1.10/24"},
			}},
		})
		sut.ResourcesDeps().ProcessAgentNetworkListed(&agentmgr.AgentConn{AssetID: "node-3"}, agentmgr.Message{
			Type: agentmgr.MsgNetworkListed,
			ID:   req.RequestID,
			Data: raw,
		})
	}()

	req := httptest.NewRequest(http.MethodGet, "/network/node-3", nil)
	rec := httptest.NewRecorder()
	sut.ResourcesDeps().HandleNetworks(rec, req)

	<-done

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	var response agentmgr.NetworkListedData
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Interfaces) != 1 || response.Interfaces[0].Name != "eth0" {
		t.Fatalf("unexpected response %+v", response)
	}
}

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/agentmgr"
)

func TestProcessAgentClipboardDataIgnoresUnexpectedAgent(t *testing.T) {
	sut := newTestAPIServer(t)
	bridge := &clipboardBridge{
		Ch:              make(chan agentmgr.Message, 1),
		ExpectedAgentID: "node-allowed",
	}
	sut.clipboardBridges.Store("req-1", bridge)
	defer sut.clipboardBridges.Delete("req-1")

	payload, err := json.Marshal(agentmgr.ClipboardDataPayload{
		RequestID: "req-1",
		Format:    "text",
		Text:      "hello",
	})
	if err != nil {
		t.Fatalf("marshal clipboard payload: %v", err)
	}

	sut.processAgentClipboardData(&agentmgr.AgentConn{AssetID: "node-other"}, agentmgr.Message{Data: payload})

	select {
	case <-bridge.Ch:
		t.Fatal("expected clipboard response from wrong agent to be ignored")
	default:
	}
}

func TestProcessAgentClipboardDataAcceptsExpectedAgent(t *testing.T) {
	sut := newTestAPIServer(t)
	bridge := &clipboardBridge{
		Ch:              make(chan agentmgr.Message, 1),
		ExpectedAgentID: "node-allowed",
	}
	sut.clipboardBridges.Store("req-1", bridge)
	defer sut.clipboardBridges.Delete("req-1")

	payload, err := json.Marshal(agentmgr.ClipboardDataPayload{
		RequestID: "req-1",
		Format:    "text",
		Text:      "hello",
	})
	if err != nil {
		t.Fatalf("marshal clipboard payload: %v", err)
	}

	sut.processAgentClipboardData(&agentmgr.AgentConn{AssetID: "node-allowed"}, agentmgr.Message{Data: payload})

	select {
	case msg := <-bridge.Ch:
		var got agentmgr.ClipboardDataPayload
		if err := json.Unmarshal(msg.Data, &got); err != nil {
			t.Fatalf("unmarshal delivered payload: %v", err)
		}
		if got.Text != "hello" {
			t.Fatalf("unexpected clipboard content %q", got.Text)
		}
	default:
		t.Fatal("expected clipboard response from matching agent to be delivered")
	}
}

func TestProcessAgentClipboardSetAckIgnoresUnexpectedAgent(t *testing.T) {
	sut := newTestAPIServer(t)
	bridge := &clipboardBridge{
		Ch:              make(chan agentmgr.Message, 1),
		ExpectedAgentID: "node-allowed",
	}
	sut.clipboardBridges.Store("req-ack", bridge)
	defer sut.clipboardBridges.Delete("req-ack")

	payload, err := json.Marshal(agentmgr.ClipboardSetAckData{
		RequestID: "req-ack",
	})
	if err != nil {
		t.Fatalf("marshal clipboard ack payload: %v", err)
	}

	sut.processAgentClipboardSetAck(&agentmgr.AgentConn{AssetID: "node-other"}, agentmgr.Message{Data: payload})

	select {
	case <-bridge.Ch:
		t.Fatal("expected clipboard ack from wrong agent to be ignored")
	default:
	}
}

func TestProcessAgentClipboardSetAckAcceptsExpectedAgent(t *testing.T) {
	sut := newTestAPIServer(t)
	bridge := &clipboardBridge{
		Ch:              make(chan agentmgr.Message, 1),
		ExpectedAgentID: "node-allowed",
	}
	sut.clipboardBridges.Store("req-ack", bridge)
	defer sut.clipboardBridges.Delete("req-ack")

	payload, err := json.Marshal(agentmgr.ClipboardSetAckData{
		RequestID: "req-ack",
	})
	if err != nil {
		t.Fatalf("marshal clipboard ack payload: %v", err)
	}

	sut.processAgentClipboardSetAck(&agentmgr.AgentConn{AssetID: "node-allowed"}, agentmgr.Message{Data: payload})

	select {
	case msg := <-bridge.Ch:
		var got agentmgr.ClipboardSetAckData
		if err := json.Unmarshal(msg.Data, &got); err != nil {
			t.Fatalf("unmarshal delivered ack payload: %v", err)
		}
		if got.RequestID != "req-ack" {
			t.Fatalf("unexpected clipboard ack request id %q", got.RequestID)
		}
	default:
		t.Fatal("expected clipboard ack from matching agent to be delivered")
	}
}

func TestHandleClipboardGetBridgesAgentSuccess(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()

	serverConn, clientConn, cleanup := createWSPairForNetworkTest(t)
	defer cleanup()

	sut.agentMgr.Register(agentmgr.NewAgentConn(serverConn, "node-1", "linux"))
	defer sut.agentMgr.Unregister("node-1")

	done := make(chan struct{})
	go func() {
		defer close(done)

		var outbound agentmgr.Message
		if err := clientConn.ReadJSON(&outbound); err != nil {
			t.Errorf("read outbound clipboard get: %v", err)
			return
		}
		if outbound.Type != agentmgr.MsgClipboardGet {
			t.Errorf("outbound type=%q, want %q", outbound.Type, agentmgr.MsgClipboardGet)
			return
		}

		var req agentmgr.ClipboardGetData
		if err := json.Unmarshal(outbound.Data, &req); err != nil {
			t.Errorf("decode clipboard get payload: %v", err)
			return
		}
		if req.RequestID == "" {
			t.Error("expected clipboard get request id")
			return
		}
		if outbound.ID != req.RequestID {
			t.Errorf("outbound id=%q, want request id %q", outbound.ID, req.RequestID)
		}
		if req.Format != "text" {
			t.Errorf("format=%q, want text", req.Format)
		}

		raw, _ := json.Marshal(agentmgr.ClipboardDataPayload{
			RequestID: req.RequestID,
			Format:    "text",
			Text:      "hello from agent",
		})
		sut.processAgentClipboardData(&agentmgr.AgentConn{AssetID: "node-1"}, agentmgr.Message{
			Type: agentmgr.MsgClipboardData,
			Data: raw,
		})
	}()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/node-1/clipboard/get", bytes.NewBufferString(`{"format":"text"}`))
	rec := httptest.NewRecorder()
	sut.handleClipboardRoutes(rec, req)

	<-done

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	var response agentmgr.ClipboardDataPayload
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode clipboard get response: %v", err)
	}
	if response.RequestID == "" {
		t.Fatalf("expected response request id, got %+v", response)
	}
	if response.Format != "text" || response.Text != "hello from agent" {
		t.Fatalf("unexpected response payload %+v", response)
	}
}

func TestHandleClipboardSetPropagatesAgentError(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()

	serverConn, clientConn, cleanup := createWSPairForNetworkTest(t)
	defer cleanup()

	sut.agentMgr.Register(agentmgr.NewAgentConn(serverConn, "node-2", "linux"))
	defer sut.agentMgr.Unregister("node-2")

	done := make(chan struct{})
	go func() {
		defer close(done)

		var outbound agentmgr.Message
		if err := clientConn.ReadJSON(&outbound); err != nil {
			t.Errorf("read outbound clipboard set: %v", err)
			return
		}
		if outbound.Type != agentmgr.MsgClipboardSet {
			t.Errorf("outbound type=%q, want %q", outbound.Type, agentmgr.MsgClipboardSet)
			return
		}

		var req agentmgr.ClipboardSetData
		if err := json.Unmarshal(outbound.Data, &req); err != nil {
			t.Errorf("decode clipboard set payload: %v", err)
			return
		}
		if req.RequestID == "" {
			t.Error("expected clipboard set request id")
			return
		}
		if outbound.ID != req.RequestID {
			t.Errorf("outbound id=%q, want request id %q", outbound.ID, req.RequestID)
		}
		if req.Format != "text" {
			t.Errorf("format=%q, want text", req.Format)
		}
		if req.Text != "hello from hub" {
			t.Errorf("text=%q, want hello from hub", req.Text)
		}

		raw, _ := json.Marshal(agentmgr.ClipboardSetAckData{
			RequestID: req.RequestID,
			OK:        false,
			Error:     "xclip failed",
		})
		sut.processAgentClipboardSetAck(&agentmgr.AgentConn{AssetID: "node-2"}, agentmgr.Message{
			Type: agentmgr.MsgClipboardSetAck,
			Data: raw,
		})
	}()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/node-2/clipboard/set", bytes.NewBufferString(`{"format":"text","text":"hello from hub"}`))
	rec := httptest.NewRecorder()
	sut.handleClipboardRoutes(rec, req)

	<-done

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "xclip failed") {
		t.Fatalf("expected xclip failed error, got %s", rec.Body.String())
	}
}

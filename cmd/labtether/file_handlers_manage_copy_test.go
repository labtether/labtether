package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
)

func TestHandleFileCopyRejectsInvalidMethod(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()

	req := httptest.NewRequest(http.MethodGet, "/files/node-1/copy?src_path=%2Ftmp%2Fa&dst_path=%2Ftmp%2Fb", nil)
	rec := httptest.NewRecorder()
	sut.handleFileCopy(rec, req, "node-1")

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleFileCopyRejectsMissingPaths(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()

	req := httptest.NewRequest(http.MethodPost, "/files/node-1/copy?src_path=%2Ftmp%2Fa", nil)
	rec := httptest.NewRecorder()
	sut.handleFileCopy(rec, req, "node-1")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "src_path and dst_path are required") {
		t.Fatalf("expected missing path error, got %s", rec.Body.String())
	}
}

func TestHandleFileCopyBridgesAgentSuccess(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()

	serverConn, clientConn, cleanup := createWSPairForNetworkTest(t)
	defer cleanup()

	sut.agentMgr.Register(agentmgr.NewAgentConn(serverConn, "node-copy", "linux"))
	defer sut.agentMgr.Unregister("node-copy")

	done := make(chan struct{})
	go func() {
		defer close(done)
		var outbound agentmgr.Message
		if err := clientConn.ReadJSON(&outbound); err != nil {
			t.Errorf("read outbound: %v", err)
			return
		}
		if outbound.Type != agentmgr.MsgFileCopy {
			t.Errorf("outbound type=%q, want %q", outbound.Type, agentmgr.MsgFileCopy)
			return
		}

		var copyReq agentmgr.FileCopyData
		if err := json.Unmarshal(outbound.Data, &copyReq); err != nil {
			t.Errorf("decode file copy payload: %v", err)
			return
		}
		if copyReq.SrcPath != "/tmp/src.txt" {
			t.Errorf("src_path=%q, want /tmp/src.txt", copyReq.SrcPath)
		}
		if copyReq.DstPath != "/tmp/dst.txt" {
			t.Errorf("dst_path=%q, want /tmp/dst.txt", copyReq.DstPath)
		}

		raw, _ := json.Marshal(agentmgr.FileResultData{
			RequestID: copyReq.RequestID,
			OK:        true,
		})
		sut.processAgentFileResult(&agentmgr.AgentConn{AssetID: "node-copy"}, agentmgr.Message{
			Type: agentmgr.MsgFileResult,
			ID:   copyReq.RequestID,
			Data: raw,
		})
	}()

	req := httptest.NewRequest(http.MethodPost, "/files/node-copy/copy?src_path=%2Ftmp%2Fsrc.txt&dst_path=%2Ftmp%2Fdst.txt", nil)
	rec := httptest.NewRecorder()
	sut.handleFileCopy(rec, req, "node-copy")

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for agent bridge")
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	var response agentmgr.FileResultData
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !response.OK {
		t.Fatalf("expected ok=true, got %+v", response)
	}
}

func TestHandleFileCopyPropagatesAgentError(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()

	serverConn, clientConn, cleanup := createWSPairForNetworkTest(t)
	defer cleanup()

	sut.agentMgr.Register(agentmgr.NewAgentConn(serverConn, "node-copy-error", "linux"))
	defer sut.agentMgr.Unregister("node-copy-error")

	done := make(chan struct{})
	go func() {
		defer close(done)
		var outbound agentmgr.Message
		if err := clientConn.ReadJSON(&outbound); err != nil {
			t.Errorf("read outbound: %v", err)
			return
		}

		var copyReq agentmgr.FileCopyData
		if err := json.Unmarshal(outbound.Data, &copyReq); err != nil {
			t.Errorf("decode file copy payload: %v", err)
			return
		}

		raw, _ := json.Marshal(agentmgr.FileResultData{
			RequestID: copyReq.RequestID,
			OK:        false,
			Error:     "destination already exists",
		})
		sut.processAgentFileResult(&agentmgr.AgentConn{AssetID: "node-copy-error"}, agentmgr.Message{
			Type: agentmgr.MsgFileResult,
			ID:   copyReq.RequestID,
			Data: raw,
		})
	}()

	req := httptest.NewRequest(http.MethodPost, "/files/node-copy-error/copy?src_path=%2Ftmp%2Fsrc.txt&dst_path=%2Ftmp%2Fdst.txt", nil)
	rec := httptest.NewRecorder()
	sut.handleFileCopy(rec, req, "node-copy-error")

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for agent bridge")
	}

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "destination already exists") {
		t.Fatalf("expected destination error, got %s", rec.Body.String())
	}
}

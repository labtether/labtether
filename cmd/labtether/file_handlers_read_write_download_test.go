package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
)

func TestHandleFileDownloadFirstPayloadErrorReturnsBadRequest(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()

	serverConn, clientConn, cleanup := createWSPairForNetworkTest(t)
	defer cleanup()

	sut.agentMgr.Register(agentmgr.NewAgentConn(serverConn, "node-download-error", "linux"))
	defer sut.agentMgr.Unregister("node-download-error")

	done := make(chan struct{})
	go func() {
		defer close(done)

		var outbound agentmgr.Message
		if err := clientConn.ReadJSON(&outbound); err != nil {
			t.Errorf("read outbound: %v", err)
			return
		}
		if outbound.Type != agentmgr.MsgFileRead {
			t.Errorf("outbound type=%q, want %q", outbound.Type, agentmgr.MsgFileRead)
			return
		}

		var readReq agentmgr.FileReadData
		if err := json.Unmarshal(outbound.Data, &readReq); err != nil {
			t.Errorf("decode file read payload: %v", err)
			return
		}
		if readReq.Path != "/tmp/bad.txt" {
			t.Errorf("path=%q, want /tmp/bad.txt", readReq.Path)
		}

		raw, _ := json.Marshal(agentmgr.FileDataPayload{
			RequestID: readReq.RequestID,
			Error:     "permission denied",
		})
		sut.processAgentFileData(&agentmgr.AgentConn{AssetID: "node-download-error"}, agentmgr.Message{
			Type: agentmgr.MsgFileData,
			ID:   readReq.RequestID,
			Data: raw,
		})
	}()

	req := httptest.NewRequest(http.MethodGet, "/files/node-download-error/download?path=%2Ftmp%2Fbad.txt", nil)
	rec := httptest.NewRecorder()
	sut.handleFileDownload(rec, req, "node-download-error")

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for agent bridge")
	}

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "permission denied") {
		t.Fatalf("expected permission denied error, got %s", rec.Body.String())
	}
	if got := rec.Header().Get("Content-Disposition"); got != "" {
		t.Fatalf("unexpected content disposition on error: %q", got)
	}
}

func TestHandleFileDownloadFirstPayloadTimeoutReturnsGatewayTimeout(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()

	serverConn, clientConn, cleanup := createWSPairForNetworkTest(t)
	defer cleanup()

	sut.agentMgr.Register(agentmgr.NewAgentConn(serverConn, "node-download-timeout", "linux"))
	defer sut.agentMgr.Unregister("node-download-timeout")

	outboundSent := make(chan struct{})
	go func() {
		defer close(outboundSent)
		var outbound agentmgr.Message
		if err := clientConn.ReadJSON(&outbound); err != nil {
			t.Errorf("read outbound: %v", err)
			return
		}
		if outbound.Type != agentmgr.MsgFileRead {
			t.Errorf("outbound type=%q, want %q", outbound.Type, agentmgr.MsgFileRead)
		}
	}()

	req := httptest.NewRequest(http.MethodGet, "/files/node-download-timeout/download?path=%2Ftmp%2Fslow.txt", nil)
	rec := httptest.NewRecorder()
	sut.handleFileDownloadWithTimeout(rec, req, "node-download-timeout", 25*time.Millisecond)

	select {
	case <-outboundSent:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for outbound read request")
	}

	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "agent did not respond in time") {
		t.Fatalf("expected timeout error, got %s", rec.Body.String())
	}
	if got := rec.Header().Get("Content-Disposition"); got != "" {
		t.Fatalf("unexpected content disposition on timeout: %q", got)
	}
}

func TestHandleFileDownloadStreamsSuccessUnchanged(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()

	serverConn, clientConn, cleanup := createWSPairForNetworkTest(t)
	defer cleanup()

	sut.agentMgr.Register(agentmgr.NewAgentConn(serverConn, "node-download-ok", "linux"))
	defer sut.agentMgr.Unregister("node-download-ok")

	done := make(chan struct{})
	go func() {
		defer close(done)

		var outbound agentmgr.Message
		if err := clientConn.ReadJSON(&outbound); err != nil {
			t.Errorf("read outbound: %v", err)
			return
		}

		var readReq agentmgr.FileReadData
		if err := json.Unmarshal(outbound.Data, &readReq); err != nil {
			t.Errorf("decode file read payload: %v", err)
			return
		}

		firstRaw, _ := json.Marshal(agentmgr.FileDataPayload{
			RequestID: readReq.RequestID,
			Data:      base64.StdEncoding.EncodeToString([]byte("hello ")),
			Offset:    0,
			Done:      false,
		})
		sut.processAgentFileData(&agentmgr.AgentConn{AssetID: "node-download-ok"}, agentmgr.Message{
			Type: agentmgr.MsgFileData,
			ID:   readReq.RequestID,
			Data: firstRaw,
		})

		secondRaw, _ := json.Marshal(agentmgr.FileDataPayload{
			RequestID: readReq.RequestID,
			Data:      base64.StdEncoding.EncodeToString([]byte("world")),
			Offset:    6,
			Done:      true,
		})
		sut.processAgentFileData(&agentmgr.AgentConn{AssetID: "node-download-ok"}, agentmgr.Message{
			Type: agentmgr.MsgFileData,
			ID:   readReq.RequestID,
			Data: secondRaw,
		})
	}()

	req := httptest.NewRequest(http.MethodGet, "/files/node-download-ok/download?path=%2Ftmp%2Fhello.txt", nil)
	rec := httptest.NewRecorder()
	sut.handleFileDownloadWithTimeout(rec, req, "node-download-ok", 2*time.Second)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for agent bridge")
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "hello world" {
		t.Fatalf("unexpected body %q", rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/octet-stream" {
		t.Fatalf("content type=%q, want application/octet-stream", got)
	}
	if got := rec.Header().Get("Content-Disposition"); !strings.Contains(got, `filename="hello.txt"`) {
		t.Fatalf("content disposition=%q, want filename=hello.txt", got)
	}
}

func TestHandleFileDownloadReturnsExplicitFailureOnMidStreamAgentError(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()

	serverConn, clientConn, cleanup := createWSPairForNetworkTest(t)
	defer cleanup()

	sut.agentMgr.Register(agentmgr.NewAgentConn(serverConn, "node-download-midstream", "linux"))
	defer sut.agentMgr.Unregister("node-download-midstream")

	done := make(chan struct{})
	go func() {
		defer close(done)

		var outbound agentmgr.Message
		if err := clientConn.ReadJSON(&outbound); err != nil {
			t.Errorf("read outbound: %v", err)
			return
		}

		var readReq agentmgr.FileReadData
		if err := json.Unmarshal(outbound.Data, &readReq); err != nil {
			t.Errorf("decode file read payload: %v", err)
			return
		}

		firstRaw, _ := json.Marshal(agentmgr.FileDataPayload{
			RequestID: readReq.RequestID,
			Data:      base64.StdEncoding.EncodeToString([]byte("partial")),
			Offset:    0,
			Done:      false,
		})
		sut.processAgentFileData(&agentmgr.AgentConn{AssetID: "node-download-midstream"}, agentmgr.Message{
			Type: agentmgr.MsgFileData,
			ID:   readReq.RequestID,
			Data: firstRaw,
		})

		secondRaw, _ := json.Marshal(agentmgr.FileDataPayload{
			RequestID: readReq.RequestID,
			Error:     "disk read failed",
			Done:      true,
		})
		sut.processAgentFileData(&agentmgr.AgentConn{AssetID: "node-download-midstream"}, agentmgr.Message{
			Type: agentmgr.MsgFileData,
			ID:   readReq.RequestID,
			Data: secondRaw,
		})
	}()

	req := httptest.NewRequest(http.MethodGet, "/files/node-download-midstream/download?path=%2Ftmp%2Fbroken.txt", nil)
	rec := httptest.NewRecorder()
	sut.handleFileDownloadWithTimeout(rec, req, "node-download-midstream", 2*time.Second)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for agent bridge")
	}

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "partial") {
		t.Fatalf("expected no partial payload to be returned, got %q", rec.Body.String())
	}
}

func TestDeliverFileResponseWaitsForBackpressuredConsumer(t *testing.T) {
	sut := newTestAPIServer(t)

	const requestID = "req-backpressure"
	bridge := newFileBridge(1, "node-download-test")
	sut.fileBridges.Store(requestID, bridge)
	defer bridge.Close()
	defer sut.fileBridges.Delete(requestID)

	firstRaw, _ := json.Marshal(agentmgr.FileDataPayload{
		RequestID: requestID,
		Data:      base64.StdEncoding.EncodeToString([]byte("one")),
	})
	secondRaw, _ := json.Marshal(agentmgr.FileDataPayload{
		RequestID: requestID,
		Data:      base64.StdEncoding.EncodeToString([]byte("two")),
	})

	bridge.Ch <- agentmgr.Message{Type: agentmgr.MsgFileData, ID: requestID, Data: firstRaw}

	delivered := make(chan struct{})
	go func() {
		defer close(delivered)
		sut.deliverFileResponse(requestID, agentmgr.Message{
			Type: agentmgr.MsgFileData,
			ID:   requestID,
			Data: secondRaw,
		})
	}()

	select {
	case <-delivered:
		t.Fatal("expected deliverFileResponse to wait for channel capacity instead of dropping the chunk")
	case <-time.After(50 * time.Millisecond):
	}

	select {
	case <-bridge.Ch:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out draining first buffered chunk")
	}

	select {
	case <-delivered:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for second chunk delivery")
	}

	select {
	case msg := <-bridge.Ch:
		var payload agentmgr.FileDataPayload
		if err := json.Unmarshal(msg.Data, &payload); err != nil {
			t.Fatalf("decode delivered payload: %v", err)
		}
		decoded, err := decodeFileDownloadChunk(payload)
		if err != nil {
			t.Fatalf("decode chunk contents: %v", err)
		}
		if string(decoded) != "two" {
			t.Fatalf("expected second chunk payload to be preserved, got %q", string(decoded))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for preserved backpressured chunk")
	}
}

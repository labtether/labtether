package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labtether/labtether/internal/agentmgr"
)

func TestHandleDesktopDiagnoseRequestRejectsNonPOST(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()

	req := httptest.NewRequest(http.MethodGet, "/desktop/diagnose/node-1", nil)
	rec := httptest.NewRecorder()
	sut.handleDesktopDiagnoseRequest(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleDesktopDiagnoseRequestRejectsMissingAssetID(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()

	req := httptest.NewRequest(http.MethodPost, "/desktop/diagnose/", nil)
	rec := httptest.NewRecorder()
	sut.handleDesktopDiagnoseRequest(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleDesktopDiagnoseRequestRejectsDisconnectedAgent(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()

	req := httptest.NewRequest(http.MethodPost, "/desktop/diagnose/not-connected", nil)
	rec := httptest.NewRecorder()
	sut.handleDesktopDiagnoseRequest(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleDesktopDiagnoseRequestBridgesAgentSuccess(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()

	serverConn, clientConn, cleanup := createWSPairForNetworkTest(t)
	defer cleanup()

	sut.agentMgr.Register(agentmgr.NewAgentConn(serverConn, "diag-node-1", "linux"))
	defer sut.agentMgr.Unregister("diag-node-1")

	done := make(chan struct{})
	go func() {
		defer close(done)

		var outbound agentmgr.Message
		if err := clientConn.ReadJSON(&outbound); err != nil {
			t.Errorf("read outbound diagnose request: %v", err)
			return
		}
		if outbound.Type != agentmgr.MsgDesktopDiagnose {
			t.Errorf("outbound type=%q, want %q", outbound.Type, agentmgr.MsgDesktopDiagnose)
			return
		}

		var req agentmgr.DesktopDiagnosticRequest
		if err := json.Unmarshal(outbound.Data, &req); err != nil {
			t.Errorf("decode diagnose request payload: %v", err)
			return
		}
		if req.RequestID == "" {
			t.Error("expected non-empty request_id in diagnose request")
			return
		}

		raw, _ := json.Marshal(agentmgr.DesktopDiagnosticData{
			RequestID:       req.RequestID,
			XvfbRunning:     true,
			XvfbDisplays:    []string{":99"},
			WebRTCAvailable: true,
			WebRTCReason:    "all good",
		})
		sut.processAgentDesktopDiagnosed(&agentmgr.AgentConn{AssetID: "diag-node-1"}, agentmgr.Message{
			Type: agentmgr.MsgDesktopDiagnosed,
			Data: raw,
		})
	}()

	req := httptest.NewRequest(http.MethodPost, "/desktop/diagnose/diag-node-1", nil)
	rec := httptest.NewRecorder()
	sut.handleDesktopDiagnoseRequest(rec, req)

	<-done

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result agentmgr.DesktopDiagnosticData
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode diagnose response: %v", err)
	}
	if !result.XvfbRunning {
		t.Error("expected XvfbRunning to be true")
	}
	if len(result.XvfbDisplays) != 1 || result.XvfbDisplays[0] != ":99" {
		t.Errorf("unexpected XvfbDisplays: %v", result.XvfbDisplays)
	}
	if !result.WebRTCAvailable {
		t.Error("expected WebRTCAvailable to be true")
	}
}

func TestProcessAgentDesktopDiagnosedIgnoresUnknownRequestID(t *testing.T) {
	sut := newTestAPIServer(t)

	raw, _ := json.Marshal(agentmgr.DesktopDiagnosticData{
		RequestID:   "req-unknown",
		XvfbRunning: true,
	})

	// Should not panic — waiter doesn't exist, message is silently dropped.
	sut.processAgentDesktopDiagnosed(&agentmgr.AgentConn{AssetID: "any-node"}, agentmgr.Message{
		Type: agentmgr.MsgDesktopDiagnosed,
		Data: raw,
	})
}

func TestProcessAgentDesktopDiagnosedDeliversToChan(t *testing.T) {
	sut := newTestAPIServer(t)

	ch := make(chan agentmgr.DesktopDiagnosticData, 1)
	sut.desktopDiagnosticWaiters.Store("req-deliver", ch)
	defer sut.desktopDiagnosticWaiters.Delete("req-deliver")

	raw, _ := json.Marshal(agentmgr.DesktopDiagnosticData{
		RequestID:   "req-deliver",
		XvfbRunning: true,
	})
	sut.processAgentDesktopDiagnosed(&agentmgr.AgentConn{AssetID: "any-node"}, agentmgr.Message{
		Type: agentmgr.MsgDesktopDiagnosed,
		Data: raw,
	})

	select {
	case result := <-ch:
		if !result.XvfbRunning {
			t.Errorf("expected XvfbRunning=true in delivered data")
		}
	default:
		t.Fatal("expected diagnostic data to be delivered to channel")
	}
}

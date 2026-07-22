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

func TestHandlePackageActionRejectsOptionInjectionWithoutForwarding(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()

	serverConn, clientConn, cleanup := createWSPairForNetworkTest(t)
	defer cleanup()

	sut.agentMgr.Register(agentmgr.NewAgentConn(serverConn, "node-pkg-security", "linux"))
	defer sut.agentMgr.Unregister("node-pkg-security")

	request := httptest.NewRequest(
		http.MethodPost,
		"/packages/node-pkg-security/install",
		strings.NewReader(`{"packages":["curl","-o","APT::Update::Pre-Invoke::=/bin/sh"]}`),
	)
	responseCh := make(chan *httptest.ResponseRecorder, 1)
	if err := clientConn.SetReadDeadline(time.Now().Add(250 * time.Millisecond)); err != nil {
		t.Fatalf("set websocket read deadline: %v", err)
	}
	go func() {
		recorder := httptest.NewRecorder()
		sut.handlePackages(recorder, request)
		responseCh <- recorder
	}()

	var outbound agentmgr.Message
	readErr := clientConn.ReadJSON(&outbound)
	if readErr == nil {
		// Release the handler if a regression forwarded the request, avoiding a
		// ten-minute package-action timeout in the failing test.
		var forwarded agentmgr.PackageActionData
		if err := json.Unmarshal(outbound.Data, &forwarded); err == nil {
			raw, _ := json.Marshal(agentmgr.PackageResultData{
				RequestID: forwarded.RequestID,
				Error:     "unexpected forwarded request",
			})
			sut.processAgentPackageResult(&agentmgr.AgentConn{AssetID: "node-pkg-security"}, agentmgr.Message{
				Type: agentmgr.MsgPackageResult,
				ID:   forwarded.RequestID,
				Data: raw,
			})
		}
	}

	select {
	case recorder := <-responseCh:
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
		}
	case <-time.After(time.Second):
		t.Fatal("package action handler did not return")
	}
	if readErr == nil {
		t.Fatalf("unsafe package request was forwarded to agent: %+v", outbound)
	}
}

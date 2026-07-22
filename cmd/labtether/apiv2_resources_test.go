// cmd/labtether/apiv2_resources_test.go
package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/assets"
)

func TestHandleV2AssetFiles_ScopeDenied(t *testing.T) {
	s := newTestAPIServer(t)

	req := httptest.NewRequest("GET", "/api/v2/assets/srv1/files?path=/", nil)
	ctx := contextWithPrincipal(req.Context(), "apikey:k1", "operator")
	ctx = contextWithScopes(ctx, []string{"assets:read"}) // no files:read
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	s.handleV2AssetFiles(rec, req, "srv1", "")

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestHandleV2AssetProcesses_ScopeDenied(t *testing.T) {
	s := newTestAPIServer(t)

	req := httptest.NewRequest("GET", "/api/v2/assets/srv1/processes", nil)
	ctx := contextWithPrincipal(req.Context(), "apikey:k1", "operator")
	ctx = contextWithScopes(ctx, []string{"assets:read"}) // no processes:read
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	s.handleV2AssetProcesses(rec, req, "srv1")

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestHandleV2AssetServices_ScopeDenied(t *testing.T) {
	s := newTestAPIServer(t)

	req := httptest.NewRequest("GET", "/api/v2/assets/srv1/services", nil)
	ctx := contextWithPrincipal(req.Context(), "apikey:k1", "operator")
	ctx = contextWithScopes(ctx, []string{"assets:read"}) // no services:read
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	s.handleV2AssetServices(rec, req, "srv1")

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestHandleV2AssetServiceActionAdaptsPathToLegacyPayload(t *testing.T) {
	s := newTestAPIServer(t)
	s.agentMgr = agentmgr.NewManager()
	serverConn, clientConn, cleanup := createWSPairForNetworkTest(t)
	defer cleanup()
	s.agentMgr.Register(agentmgr.NewAgentConn(serverConn, "srv1", "linux"))
	defer s.agentMgr.Unregister("srv1")

	done := make(chan struct{})
	go func() {
		defer close(done)
		var outbound agentmgr.Message
		if err := clientConn.ReadJSON(&outbound); err != nil {
			t.Errorf("read service action: %v", err)
			return
		}
		if outbound.Type != agentmgr.MsgServiceAction {
			t.Errorf("message type=%q, want %q", outbound.Type, agentmgr.MsgServiceAction)
			return
		}
		var action agentmgr.ServiceActionData
		if err := json.Unmarshal(outbound.Data, &action); err != nil {
			t.Errorf("decode service action: %v", err)
			return
		}
		if action.Service != "labtether-qa-fixture" || action.Action != "restart" {
			t.Errorf("service action=%+v, want path-derived fixture restart", action)
		}
		data, _ := json.Marshal(agentmgr.ServiceResultData{RequestID: action.RequestID, OK: true})
		s.processAgentServiceResult(&agentmgr.AgentConn{AssetID: "srv1"}, agentmgr.Message{
			Type: agentmgr.MsgServiceResult,
			ID:   action.RequestID,
			Data: data,
		})
	}()

	// A conflicting legacy-shaped body must not override the V2 path contract.
	req := httptest.NewRequest(http.MethodPost, "/api/v2/assets/srv1/services/labtether-qa-fixture/restart", strings.NewReader(`{"service":"ssh"}`))
	ctx := contextWithScopes(req.Context(), []string{"services:write"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	s.handleV2AssetServiceActions(rec, req, "srv1", "labtether-qa-fixture/restart")
	<-done

	if rec.Code != http.StatusOK {
		t.Fatalf("service action status=%d body=%s, want 200", rec.Code, rec.Body.String())
	}
}

func TestHandleV2AssetProcesses_ScopeAllowed(t *testing.T) {
	s := newTestAPIServer(t)
	_, err := s.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID:  "srv1",
		Name:     "srv1",
		Status:   "online",
		Platform: "linux",
		Source:   "agent",
		Type:     "host",
	})
	if err != nil {
		t.Fatalf("failed to create test asset: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/v2/assets/srv1/processes", nil)
	ctx := contextWithPrincipal(req.Context(), "apikey:k1", "operator")
	ctx = contextWithScopes(ctx, []string{"processes:read"})
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	s.handleV2AssetProcesses(rec, req, "srv1")

	// Will fail with agent not connected (not 403), which is the right behavior
	// — the scope check passed.
	if rec.Code == http.StatusForbidden {
		t.Fatal("scope check should have passed")
	}
}

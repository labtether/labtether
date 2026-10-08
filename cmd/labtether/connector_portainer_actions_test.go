package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/labtether/labtether/internal/connectors/portainer"
	"github.com/labtether/labtether/internal/connectorsdk"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleConnectorActionsPortainerRouteErrors(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	t.Run("portainer test method not allowed", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.connectorRegistry.Register(&mockPortainerConnector{})

		req := httptest.NewRequest(http.MethodGet, "/connectors/portainer/test", nil)
		rec := httptest.NewRecorder()
		sut.handleConnectorActions(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", rec.Code)
		}
		assertErrorBodyContains(t, rec.Body.Bytes(), "method not allowed")
	})

	t.Run("connector path not found", func(t *testing.T) {
		sut := newTestAPIServer(t)
		req := httptest.NewRequest(http.MethodGet, "/connectors/", nil)
		rec := httptest.NewRecorder()
		sut.handleConnectorActions(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
		assertErrorBodyContains(t, rec.Body.Bytes(), "connector path not found")
	})

	t.Run("invalid connector path", func(t *testing.T) {
		sut := newTestAPIServer(t)
		req := httptest.NewRequest(http.MethodGet, "/connectors/portainer", nil)
		rec := httptest.NewRecorder()
		sut.handleConnectorActions(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
		assertErrorBodyContains(t, rec.Body.Bytes(), "invalid connector path")
	})

	t.Run("connector not registered", func(t *testing.T) {
		sut := newTestAPIServer(t)
		req := httptest.NewRequest(http.MethodGet, "/connectors/portainer/actions", nil)
		rec := httptest.NewRecorder()
		sut.handleConnectorActions(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
		assertErrorBodyContains(t, rec.Body.Bytes(), "connector not registered")
	})

	t.Run("unknown connector action", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.connectorRegistry.Register(&mockPortainerConnector{})

		req := httptest.NewRequest(http.MethodGet, "/connectors/portainer/not-real", nil)
		rec := httptest.NewRecorder()
		sut.handleConnectorActions(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
		assertErrorBodyContains(t, rec.Body.Bytes(), "unknown connector action")
	})
}

func TestHandleConnectorActionsPortainerDiscover(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	t.Run("method not allowed", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.connectorRegistry.Register(&mockPortainerConnector{})

		req := httptest.NewRequest(http.MethodPost, "/connectors/portainer/discover", nil)
		rec := httptest.NewRecorder()
		sut.handleConnectorActions(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", rec.Code)
		}
		assertErrorBodyContains(t, rec.Body.Bytes(), "method not allowed")
	})

	t.Run("discover failed", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.connectorRegistry.Register(&mockPortainerConnector{
			discoverFn: func(context.Context) ([]connectorsdk.Asset, error) {
				return nil, errors.New("discover boom")
			},
		})

		req := httptest.NewRequest(http.MethodGet, "/connectors/portainer/discover", nil)
		rec := httptest.NewRecorder()
		sut.handleConnectorActions(rec, req)

		if rec.Code != http.StatusBadGateway {
			t.Fatalf("expected 502, got %d", rec.Code)
		}
		assertErrorBodyContains(t, rec.Body.Bytes(), "An internal error occurred.")
	})

	t.Run("discover success", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.connectorRegistry.Register(&mockPortainerConnector{
			discoverFn: func(context.Context) ([]connectorsdk.Asset, error) {
				return []connectorsdk.Asset{
					{ID: "portainer-endpoint-1", Type: "container-host", Name: "endpoint-1", Source: "portainer", Metadata: map[string]string{"endpoint_id": "1"}},
					{ID: "portainer-container-1-abcd", Type: "container", Name: "nginx", Source: "portainer", Metadata: map[string]string{"endpoint_id": "1", "container_id": "abcd"}},
				}, nil
			},
		})

		req := httptest.NewRequest(http.MethodGet, "/connectors/portainer/discover", nil)
		rec := httptest.NewRecorder()
		sut.handleConnectorActions(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}
		var payload struct {
			Assets         []connectorsdk.Asset `json:"assets"`
			Relationships  []map[string]any     `json:"relationships"`
			CapabilitySets []map[string]any     `json:"capability_sets"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if len(payload.Assets) != 2 || payload.Assets[0].ID != "portainer-endpoint-1" {
			t.Fatalf("unexpected discover payload: %#v", payload.Assets)
		}
		if len(payload.Relationships) == 0 {
			t.Fatalf("expected synthesized relationships in discover payload")
		}
		if len(payload.CapabilitySets) == 0 {
			t.Fatalf("expected synthesized capability_sets in discover payload")
		}
	})
}

func TestHandleConnectorActionsPortainerHealth(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	t.Run("method not allowed", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.connectorRegistry.Register(&mockPortainerConnector{})

		req := httptest.NewRequest(http.MethodPost, "/connectors/portainer/health", nil)
		rec := httptest.NewRecorder()
		sut.handleConnectorActions(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", rec.Code)
		}
		assertErrorBodyContains(t, rec.Body.Bytes(), "method not allowed")
	})

	t.Run("health failed", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.connectorRegistry.Register(&mockPortainerConnector{
			testFn: func(context.Context) (connectorsdk.Health, error) {
				return connectorsdk.Health{}, errors.New("health boom")
			},
		})

		req := httptest.NewRequest(http.MethodGet, "/connectors/portainer/health", nil)
		rec := httptest.NewRecorder()
		sut.handleConnectorActions(rec, req)

		if rec.Code != http.StatusBadGateway {
			t.Fatalf("expected 502, got %d", rec.Code)
		}
		assertErrorBodyContains(t, rec.Body.Bytes(), "An internal error occurred.")
	})

	t.Run("health success", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.connectorRegistry.Register(&mockPortainerConnector{
			testFn: func(context.Context) (connectorsdk.Health, error) {
				return connectorsdk.Health{Status: "ok", Message: "healthy"}, nil
			},
		})

		req := httptest.NewRequest(http.MethodGet, "/connectors/portainer/health", nil)
		rec := httptest.NewRecorder()
		sut.handleConnectorActions(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}
		var payload connectorsdk.Health
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if payload.Status != "ok" || payload.Message != "healthy" {
			t.Fatalf("unexpected health payload: %#v", payload)
		}
	})
}

func TestHandleConnectorActionsPortainerActionsAndExecute(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	t.Run("actions list method not allowed", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.connectorRegistry.Register(&mockPortainerConnector{})

		req := httptest.NewRequest(http.MethodPost, "/connectors/portainer/actions", nil)
		rec := httptest.NewRecorder()
		sut.handleConnectorActions(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", rec.Code)
		}
		assertErrorBodyContains(t, rec.Body.Bytes(), "method not allowed")
	})

	t.Run("actions list success", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.connectorRegistry.Register(&mockPortainerConnector{
			actionsFn: func() []connectorsdk.ActionDescriptor {
				return []connectorsdk.ActionDescriptor{
					{ID: "container.restart", Name: "Restart Container", RequiresTarget: true},
				}
			},
		})

		req := httptest.NewRequest(http.MethodGet, "/connectors/portainer/actions", nil)
		rec := httptest.NewRecorder()
		sut.handleConnectorActions(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}
		var payload struct {
			Actions []connectorsdk.ActionDescriptor `json:"actions"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if len(payload.Actions) != 1 || payload.Actions[0].ID != "container.restart" {
			t.Fatalf("unexpected actions payload: %#v", payload.Actions)
		}
	})

	t.Run("execute method not allowed", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.connectorRegistry.Register(&mockPortainerConnector{})

		req := httptest.NewRequest(http.MethodGet, "/connectors/portainer/actions/container.restart/execute", nil)
		rec := httptest.NewRecorder()
		sut.handleConnectorActions(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", rec.Code)
		}
		assertErrorBodyContains(t, rec.Body.Bytes(), "method not allowed")
	})

	t.Run("execute invalid payload", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.connectorRegistry.Register(&mockPortainerConnector{})

		req := httptest.NewRequest(http.MethodPost, "/connectors/portainer/actions/container.restart/execute", strings.NewReader("{"))
		rec := httptest.NewRecorder()
		sut.handleConnectorActions(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		assertErrorBodyContains(t, rec.Body.Bytes(), "invalid action payload")
	})

	t.Run("execute error", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.connectorRegistry.Register(&mockPortainerConnector{
			executeFn: func(context.Context, string, connectorsdk.ActionRequest) (connectorsdk.ActionResult, error) {
				return connectorsdk.ActionResult{}, errors.New("execute boom")
			},
		})

		req := httptest.NewRequest(http.MethodPost, "/connectors/portainer/actions/container.restart/execute", strings.NewReader(`{}`))
		rec := httptest.NewRecorder()
		sut.handleConnectorActions(rec, req)

		if rec.Code != http.StatusBadGateway {
			t.Fatalf("expected 502, got %d", rec.Code)
		}
		assertErrorBodyContains(t, rec.Body.Bytes(), "An internal error occurred.")
	})

	t.Run("execute success and eof decode path", func(t *testing.T) {
		var gotActionID string
		var gotReq connectorsdk.ActionRequest

		sut := newTestAPIServer(t)
		sut.connectorRegistry.Register(&mockPortainerConnector{
			executeFn: func(_ context.Context, actionID string, req connectorsdk.ActionRequest) (connectorsdk.ActionResult, error) {
				gotActionID = actionID
				gotReq = req
				return connectorsdk.ActionResult{Status: "ok", Message: "done"}, nil
			},
		})

		req := httptest.NewRequest(http.MethodPost, "/connectors/portainer/actions/container.restart/execute", nil)
		rec := httptest.NewRecorder()
		sut.handleConnectorActions(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}
		if gotActionID != "container.restart" {
			t.Fatalf("expected actionID container.restart, got %q", gotActionID)
		}
		if gotReq.TargetID != "" || gotReq.Params != nil || gotReq.DryRun {
			t.Fatalf("expected zero action request from EOF body, got %#v", gotReq)
		}
	})

	t.Run("execute failed status maps to bad request", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.connectorRegistry.Register(&mockPortainerConnector{
			executeFn: func(context.Context, string, connectorsdk.ActionRequest) (connectorsdk.ActionResult, error) {
				return connectorsdk.ActionResult{Status: "failed", Message: "bad target"}, nil
			},
		})

		req := httptest.NewRequest(http.MethodPost, "/connectors/portainer/actions/container.restart/execute", strings.NewReader(`{"target_id":"container-1"}`))
		rec := httptest.NewRecorder()
		sut.handleConnectorActions(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
		}
	})
}

func TestHandleConnectorActionsDispatchesPortainerTest(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/system/version":
			_, _ = w.Write([]byte(`{"ServerVersion":"2.21.0"}`))
		case "/api/endpoints":
			_, _ = w.Write([]byte(`[{"Id":1,"Name":"edge"}]`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("X-API-Key") != "ptr-secret" {
			t.Fatalf("expected API key header")
		}
	}))
	defer mock.Close()

	sut := newTestAPIServer(t)
	sut.connectorRegistry.Register(portainer.New())

	payload := []byte(fmt.Sprintf(`{
		"auth_method":"api_key",
		"base_url":"%s",
		"token_id":"svc@local!automation",
		"token_secret":"ptr-secret"
	}`, mock.URL))
	req := httptest.NewRequest(http.MethodPost, "/connectors/portainer/test", bytes.NewReader(payload))
	rec := httptest.NewRecorder()
	sut.handleConnectorActions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleConnectorActionsPortainerTestRateLimit(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	sut := newTestAPIServer(t)
	sut.connectorRegistry.Register(&mockPortainerConnector{})

	for i := 0; i < 12; i++ {
		req := httptest.NewRequest(http.MethodPost, "/connectors/portainer/test", strings.NewReader(`{"auth_method":"api_key"}`))
		req.RemoteAddr = "203.0.113.41:4401"
		rec := httptest.NewRecorder()
		sut.handleConnectorActions(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("request %d: expected 400 before rate limit, got %d", i+1, rec.Code)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/connectors/portainer/test", strings.NewReader(`{"auth_method":"api_key"}`))
	req.RemoteAddr = "203.0.113.41:4401"
	rec := httptest.NewRecorder()
	sut.handleConnectorActions(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after connector test burst, got %d body=%s", rec.Code, rec.Body.String())
	}
	assertErrorBodyContains(t, rec.Body.Bytes(), "rate limit exceeded")
}

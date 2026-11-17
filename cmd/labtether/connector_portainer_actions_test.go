package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleConnectorActionsPortainerDiscover(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)
	t.Run("method not allowed", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.connectorRegistry.Register(&mockPortainerConnector{})

		req := httptest.NewRequest(http.MethodPost, "/connectors/portainer/discover", nil)
		rec := httptest.NewRecorder()
		sut.ensureCollectorsDeps().HandleConnectorActions(rec, req)

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
		sut.ensureCollectorsDeps().HandleConnectorActions(rec, req)

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
		sut.ensureCollectorsDeps().HandleConnectorActions(rec, req)

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
		sut.ensureCollectorsDeps().HandleConnectorActions(rec, req)

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
		sut.ensureCollectorsDeps().HandleConnectorActions(rec, req)

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
		sut.ensureCollectorsDeps().HandleConnectorActions(rec, req)

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
	// These cases cover HTTP response mapping only. The configured runtime
	// fixtures separately prove provider binding with real HTTP clients.
	setExecute := func(t *testing.T, sut *apiServer, execute func(context.Context, string, connectorsdk.ActionRequest) (connectorsdk.ActionResult, error)) {
		t.Helper()
		sut.connectorRegistry.Register(&mockPortainerConnector{executeFn: func(context.Context, string, connectorsdk.ActionRequest) (connectorsdk.ActionResult, error) {
			t.Fatal("configured action fell back to the startup catalog")
			return connectorsdk.ActionResult{}, nil
		}})
		sut.ensureCollectorsDeps().ConfiguredConnectorActions["portainer"] = shared.ConfiguredConnectorAction{
			Bind: func(_ context.Context, _ string, req connectorsdk.ActionRequest) (connectorsdk.ActionRequest, error) {
				return req, nil
			},
			Execute: execute,
		}
	}
	t.Run("actions list method not allowed", func(t *testing.T) {
		sut := newTestAPIServer(t)
		sut.connectorRegistry.Register(&mockPortainerConnector{})

		req := httptest.NewRequest(http.MethodPost, "/connectors/portainer/actions", nil)
		rec := httptest.NewRecorder()
		sut.ensureCollectorsDeps().HandleConnectorActions(rec, req)

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
		sut.ensureCollectorsDeps().HandleConnectorActions(rec, req)

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
		sut.ensureCollectorsDeps().HandleConnectorActions(rec, req)

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
		sut.ensureCollectorsDeps().HandleConnectorActions(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
		assertErrorBodyContains(t, rec.Body.Bytes(), "invalid action payload")
	})

	t.Run("execute error", func(t *testing.T) {
		sut := newTestAPIServer(t)
		setExecute(t, sut, func(context.Context, string, connectorsdk.ActionRequest) (connectorsdk.ActionResult, error) {
			return connectorsdk.ActionResult{}, errors.New("execute boom")
		})

		req := httptest.NewRequest(http.MethodPost, "/connectors/portainer/actions/container.restart/execute", strings.NewReader(`{}`))
		rec := httptest.NewRecorder()
		sut.ensureCollectorsDeps().HandleConnectorActions(rec, req)

		if rec.Code != http.StatusBadGateway {
			t.Fatalf("expected 502, got %d", rec.Code)
		}
		assertErrorBodyContains(t, rec.Body.Bytes(), "An internal error occurred.")
	})

	t.Run("execute success and eof decode path", func(t *testing.T) {
		var gotActionID string
		var gotReq connectorsdk.ActionRequest

		sut := newTestAPIServer(t)
		setExecute(t, sut, func(_ context.Context, actionID string, req connectorsdk.ActionRequest) (connectorsdk.ActionResult, error) {
			gotActionID = actionID
			gotReq = req
			return connectorsdk.ActionResult{Status: "ok", Message: "done"}, nil
		})

		req := httptest.NewRequest(http.MethodPost, "/connectors/portainer/actions/container.restart/execute", nil)
		rec := httptest.NewRecorder()
		sut.ensureCollectorsDeps().HandleConnectorActions(rec, req)

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
		setExecute(t, sut, func(context.Context, string, connectorsdk.ActionRequest) (connectorsdk.ActionResult, error) {
			return connectorsdk.ActionResult{Status: "failed", Message: "bad target"}, nil
		})

		req := httptest.NewRequest(http.MethodPost, "/connectors/portainer/actions/container.restart/execute", strings.NewReader(`{"target_id":"container-1"}`))
		rec := httptest.NewRecorder()
		sut.ensureCollectorsDeps().HandleConnectorActions(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
		}
	})
}

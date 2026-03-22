package mcpserver

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/labtether/labtether/internal/assets"
)

type mockAssetStore struct {
	assets []assets.Asset
}

func (m *mockAssetStore) ListAssets() ([]assets.Asset, error) { return m.assets, nil }
func (m *mockAssetStore) GetAsset(id string) (assets.Asset, bool, error) {
	for _, a := range m.assets {
		if a.ID == id {
			return a, true, nil
		}
	}
	return assets.Asset{}, false, nil
}

type mockAgentMgr struct{ connected map[string]bool }

func (m *mockAgentMgr) IsConnected(id string) bool { return m.connected[id] }

func newTestDeps() *Deps {
	return &Deps{
		AssetStore: &mockAssetStore{assets: []assets.Asset{
			{ID: "srv1", Name: "Server 1", Platform: "linux", Status: "online"},
			{ID: "srv2", Name: "Server 2", Platform: "linux", Status: "offline"},
		}},
		AgentMgr:         &mockAgentMgr{connected: map[string]bool{"srv1": true}},
		ExecuteViaAgent:  nil,
		GetScopes:        func(ctx context.Context) []string { return []string{"assets:read", "assets:exec"} },
		GetAllowedAssets: func(ctx context.Context) []string { return nil },
		GetActorID:       func(ctx context.Context) string { return "test" },
	}
}

func TestHandleWhoami(t *testing.T) {
	deps := newTestDeps()
	result, err := deps.handleWhoami(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("whoami error: %v", err)
	}
	if result.IsError {
		t.Fatal("whoami should not error")
	}
}

func TestHandleAssetsList(t *testing.T) {
	deps := newTestDeps()
	result, err := deps.handleAssetsList(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("assets_list error: %v", err)
	}
	if result.IsError {
		t.Fatal("assets_list should not error")
	}
}

func TestHandleAssetsGet(t *testing.T) {
	deps := newTestDeps()
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"asset_id": "srv1"}
	result, err := deps.handleAssetsGet(context.Background(), req)
	if err != nil {
		t.Fatalf("assets_get error: %v", err)
	}
	if result.IsError {
		t.Fatal("assets_get should not error for existing asset")
	}
}

func TestHandleAssetsGet_NotFound(t *testing.T) {
	deps := newTestDeps()
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"asset_id": "nonexistent"}
	result, err := deps.handleAssetsGet(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("should return error for nonexistent asset")
	}
}

func TestHandleExec_ScopeDenied(t *testing.T) {
	deps := newTestDeps()
	deps.GetScopes = func(ctx context.Context) []string { return []string{"assets:read"} } // no exec
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"asset_id": "srv1", "command": "uptime"}
	result, err := deps.handleExec(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("should return error for insufficient scope")
	}
}

func TestHandleExec_AssetOffline(t *testing.T) {
	deps := newTestDeps()
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"asset_id": "srv2", "command": "uptime"}
	result, err := deps.handleExec(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("should return error for offline asset")
	}
}

// --- New tool tests ---

func TestHandleServicesList_ScopeDenied(t *testing.T) {
	deps := newTestDeps()
	deps.GetScopes = func(ctx context.Context) []string { return []string{"assets:read"} }
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"asset_id": "srv1"}
	result, err := deps.handleServicesList(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("should return error for insufficient scope")
	}
}

func TestHandleServicesList_NoAgent(t *testing.T) {
	deps := newTestDeps()
	deps.GetScopes = func(ctx context.Context) []string { return []string{"services:read"} }
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"asset_id": "srv2"} // srv2 is offline/not connected
	result, err := deps.handleServicesList(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("should return error when agent is not connected")
	}
}

func TestHandleAlertsList_NilDep(t *testing.T) {
	deps := newTestDeps()
	deps.GetScopes = func(ctx context.Context) []string { return []string{"alerts:read"} }
	deps.ListAlerts = nil // not configured
	result, err := deps.handleAlertsList(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("should return error when ListAlerts is nil")
	}
}

func TestHandleGroupsList_NilDep(t *testing.T) {
	deps := newTestDeps()
	deps.GetScopes = func(ctx context.Context) []string { return []string{"groups:read"} }
	deps.ListGroups = nil // not configured
	result, err := deps.handleGroupsList(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("should return error when ListGroups is nil")
	}
}

func TestHandleAssetReboot_ScopeDenied(t *testing.T) {
	deps := newTestDeps()
	deps.GetScopes = func(ctx context.Context) []string { return []string{"assets:read"} }
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"asset_id": "srv1"}
	result, err := deps.handleAssetReboot(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("should return error for insufficient scope")
	}
}

func TestHandleSystemDisks_NoAgent(t *testing.T) {
	deps := newTestDeps()
	deps.GetScopes = func(ctx context.Context) []string { return []string{"disks:read"} }
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"asset_id": "srv2"} // srv2 not connected
	result, err := deps.handleSystemDisks(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("should return error when agent is not connected")
	}
}

func TestHandleDockerHosts_NilDep(t *testing.T) {
	deps := newTestDeps()
	deps.GetScopes = func(ctx context.Context) []string { return []string{"docker:read"} }
	deps.ListDockerHosts = nil
	result, err := deps.handleDockerHosts(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("should return error when ListDockerHosts is nil")
	}
}

func TestHandleMetricsOverview_NilDep(t *testing.T) {
	deps := newTestDeps()
	deps.GetScopes = func(ctx context.Context) []string { return []string{"metrics:read"} }
	deps.MetricsOverview = nil
	result, err := deps.handleMetricsOverview(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("should return error when MetricsOverview is nil")
	}
}

func TestHandleDockerHosts_WithData(t *testing.T) {
	deps := newTestDeps()
	deps.GetScopes = func(ctx context.Context) []string { return []string{"docker:read"} }
	deps.ListDockerHosts = func() ([]map[string]any, error) {
		return []map[string]any{{"agent_id": "agent1", "containers": 3}}, nil
	}
	result, err := deps.handleDockerHosts(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatal("should not return error when dep is configured")
	}
}

func TestHandleAlertsList_WithData(t *testing.T) {
	deps := newTestDeps()
	deps.GetScopes = func(ctx context.Context) []string { return []string{"alerts:read"} }
	deps.ListAlerts = func() ([]map[string]any, error) {
		return []map[string]any{{"id": "alert1", "status": "firing"}}, nil
	}
	result, err := deps.handleAlertsList(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatal("should not return error when dep is configured")
	}
}

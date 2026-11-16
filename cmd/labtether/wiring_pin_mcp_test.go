package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/connectors/docker"
	respkg "github.com/labtether/labtether/internal/hubapi/resources"
)

// TestWiringPinMCPInventory pins the tool, resource and prompt names of the
// MCP server as cmd builds it (buildMCPServer, which handleMCP calls once
// when the route table is built). internal/mcpserver pins its own list with
// its own test deps; this pin is the list cmd actually serves.
//
// Regenerate (then review the diff line by line):
//
//	LABTETHER_UPDATE_WIRING_PINS=1 go test -count=1 -run 'TestWiringPin' ./cmd/labtether
func TestWiringPinMCPInventory(t *testing.T) {
	srv := newMCPPinServer(t).buildMCPServer()
	var b strings.Builder
	b.WriteString("# MCP pin: names served by buildMCPServer, sorted.\n")
	b.WriteString("# Regenerate: " + wiringPinUpdateCommand + "\n")
	writeMCPPinBlock(&b, "tools", sortedKeysOf(srv.ListTools()))
	writeMCPPinBlock(&b, "resources", sortedKeysOf(srv.ListResources()))
	writeMCPPinBlock(&b, "prompts", sortedKeysOf(srv.ListPrompts()))
	compareWiringPin(t, "mcp_inventory.txt", []byte(b.String()))
}

// mcpUnwiredMarkers are the answers internal/mcpserver gives when a Deps
// field it needs is nil.
var mcpUnwiredMarkers = []string{
	"This tool requires hub dependencies that are not configured.",
	"MCP tool dependency is unavailable",
	"typed power actions are not configured",
	"MCP authorization context is unavailable",
	"MCP host data authorization is unavailable",
	"MCP asset authorization context is unavailable",
	"MCP mutation policy is unavailable",
	"command policy unavailable",
}

// mcpUnwiredAgentAnswer is internal/mcpserver's whole answer when AgentMgr is
// nil. The Hub's own handlers say "agent is not connected (last seen ...)",
// which shows the callback ran, so only an exact match counts.
const mcpUnwiredAgentAnswer = "agent is not connected"

// TestWiringPinMCPCallbacksWired calls every tool and reads every resource
// of the cmd-built MCP server as the owner, on a Hub wired like production
// (agent manager with a connected agent, docker coordinator, edge store).
// No answer may be one internal/mcpserver gives for a nil Deps field, and
// mutation tools must reach the audit callback. Agent sends fail at once (the
// probe agent's credential check refuses), so nothing leaves the process.
//
// Risk covered: a C2 move of the MCP adapter that drops a Deps field. It
// cannot see a field whose nil value changes nothing visible, such as
// GetActorID, which only feeds audit text.
func TestWiringPinMCPCallbacksWired(t *testing.T) {
	origWake := respkg.SendWakeOnLAN
	respkg.SendWakeOnLAN = func(net.HardwareAddr, string) error { return errors.New("wake disabled in wiring pin") }
	t.Cleanup(func() { respkg.SendWakeOnLAN = origWake })

	sut := newMCPPinServer(t)
	srv := sut.buildMCPServer()
	ctx := contextWithPrincipal(context.Background(), "owner", auth.RoleOwner)

	for _, name := range sortedKeysOf(srv.ListTools()) {
		tool := srv.GetTool(name)
		req := mcp.CallToolRequest{}
		req.Params.Name = name
		req.Params.Arguments = mcpPinArguments(tool.Tool)
		callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		result, err := tool.Handler(callCtx, req)
		cancel()
		text := mcpPinResultText(result, err)
		if strings.TrimSpace(text) == mcpUnwiredAgentAnswer {
			t.Errorf("tool %s answered %q: AgentMgr is not wired in buildMCPServer", name, text)
		}
		for _, marker := range mcpUnwiredMarkers {
			if strings.Contains(text, marker) {
				t.Errorf("tool %s answered %q: a Deps field it needs is not wired in buildMCPServer", name, clipRoutePinText(text))
			}
		}
	}
	for _, uri := range sortedKeysOf(srv.ListResources()) {
		req := mcp.ReadResourceRequest{}
		req.Params.URI = uri
		_, err := srv.ListResources()[uri].Handler(ctx, req)
		for _, marker := range mcpUnwiredMarkers {
			if err != nil && strings.Contains(err.Error(), marker) {
				t.Errorf("resource %s answered %q: a Deps field it needs is not wired", uri, err.Error())
			}
		}
	}

	if got := mcpPinServerVersion(t, srv, ctx); got != hubVersion() {
		t.Errorf("MCP server reports version %q, want hubVersion() %q", got, hubVersion())
	}

	events, err := sut.auditStore.List(1000, 0)
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	mcpEvents := 0
	for _, event := range events {
		if event.Type == "mcp.tool" {
			mcpEvents++
		}
	}
	if mcpEvents == 0 {
		t.Error("no mcp.tool audit event after calling every mutation tool: AuditMutation is not wired")
	}
}

// mcpPinServerVersion asks the server to initialize and returns the version
// it reports to clients.
func mcpPinServerVersion(t *testing.T, srv *server.MCPServer, ctx context.Context) string {
	t.Helper()
	raw := json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"wiring-pin","version":"0"}}}`)
	reply, err := json.Marshal(srv.HandleMessage(ctx, raw))
	if err != nil {
		t.Fatalf("encode initialize reply: %v", err)
	}
	var decoded struct {
		Result struct {
			ServerInfo struct {
				Version string `json:"version"`
			} `json:"serverInfo"`
		} `json:"result"`
	}
	if err := json.Unmarshal(reply, &decoded); err != nil {
		t.Fatalf("decode initialize reply %s: %v", reply, err)
	}
	return decoded.Result.ServerInfo.Version
}

// newMCPPinServer is the in-memory Hub plus the production pieces the MCP
// callbacks depend on. The connected agent refuses every send.
func newMCPPinServer(t *testing.T) *apiServer {
	t.Helper()
	sut := newTestAPIServer(t)
	sut.edgeStore = newMockEdgeStore()
	sut.dockerCoordinator = docker.NewCoordinator(nil)
	sut.agentMgr = agentmgr.NewManager()
	conn := agentmgr.NewAgentConn(nil, routePinAsset, "linux")
	conn.SetCredentialValidator(func() error { return errors.New("wiring pin agent refuses sends") })
	sut.agentMgr.Register(conn)
	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: routePinAsset, Type: "host", Name: routePinAsset, Source: "agent", Status: "online", Platform: "linux",
	}); err != nil {
		t.Fatalf("seed asset: %v", err)
	}
	return sut
}

// mcpPinArguments fills every declared argument with a plausible value, so a
// tool gets past input checks to the callback it depends on.
func mcpPinArguments(tool mcp.Tool) map[string]any {
	args := map[string]any{}
	for name, raw := range tool.InputSchema.Properties {
		prop, _ := raw.(map[string]any)
		if values, ok := prop["enum"].([]any); ok && len(values) > 0 {
			args[name] = values[0]
			continue
		}
		if values, ok := prop["enum"].([]string); ok && len(values) > 0 {
			args[name] = values[0]
			continue
		}
		switch prop["type"] {
		case "array":
			args[name] = []any{routePinAsset}
		case "integer", "number":
			args[name] = 1
		case "boolean":
			args[name] = false
		default:
			switch {
			case strings.Contains(name, "command"):
				args[name] = "uptime"
			case strings.Contains(name, "path"):
				args[name] = "/etc/hostname"
			default:
				args[name] = routePinAsset
			}
		}
	}
	return args
}

func mcpPinResultText(result *mcp.CallToolResult, err error) string {
	if err != nil {
		return err.Error()
	}
	if result == nil {
		return ""
	}
	var parts []string
	for _, content := range result.Content {
		if text, ok := content.(mcp.TextContent); ok {
			parts = append(parts, text.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func writeMCPPinBlock(b *strings.Builder, title string, names []string) {
	fmt.Fprintf(b, "\n== %s\ncount %d\n", title, len(names))
	for _, name := range names {
		b.WriteString(name + "\n")
	}
}

func sortedKeysOf[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

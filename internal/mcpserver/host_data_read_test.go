package mcpserver

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
)

// Viewers must not read files, process command lines or container logs
// through MCP; the role floor runs before any agent dispatch.
func TestHostDataToolsHonourReadAuthorizationBeforeDispatch(t *testing.T) {
	for _, hook := range []struct {
		name string
		fn   func(context.Context) error
	}{
		{name: "denied", fn: func(context.Context) error { return errors.New("reading host data requires operator role") }},
		{name: "unwired", fn: nil},
	} {
		t.Run(hook.name, func(t *testing.T) {
			deps := newTestDeps()
			deps.GetScopes = func(context.Context) []string { return nil }
			deps.AuthorizeHostDataRead = hook.fn
			var dispatches atomic.Int32
			deps.ListFiles = func(context.Context, string, string) (any, error) { dispatches.Add(1); return nil, nil }
			deps.ReadFile = func(context.Context, string, string) (any, error) { dispatches.Add(1); return nil, nil }
			deps.ListProcesses = func(context.Context, string) (any, error) { dispatches.Add(1); return nil, nil }
			deps.DockerContainerLogs = func(context.Context, string, string, int) (string, error) {
				dispatches.Add(1)
				return "", nil
			}
			server := NewServer(deps)
			for name, args := range map[string]map[string]any{
				"files_list":            {"asset_id": "srv1", "path": "/etc"},
				"files_read":            {"asset_id": "srv1", "path": "/etc/shadow"},
				"system_processes":      {"asset_id": "srv1"},
				"docker_container_logs": {"asset_id": "srv1", "container_id": "abc"},
			} {
				result, err := server.GetTool(name).Handler(context.Background(), toolRequest(args))
				if err != nil {
					t.Fatalf("%s: %v", name, err)
				}
				if result == nil || !result.IsError {
					t.Fatalf("%s was not denied: %#v", name, result)
				}
			}
			if got := dispatches.Load(); got != 0 {
				t.Fatalf("host data tools dispatched %d times without authorization", got)
			}
		})
	}
}

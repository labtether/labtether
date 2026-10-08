package truenas

import (
	"context"
	"github.com/gorilla/websocket"
	"github.com/labtether/labtether/internal/connectorsdk"
	"strings"
	"testing"
)

func TestConnectorExecuteActionDispatchMatrix(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	type testCase struct {
		name         string
		actionID     string
		req          connectorsdk.ActionRequest
		wantMethod   string
		wantStatus   string
		expectCalled bool
	}

	cases := []testCase{
		{
			name:       "pool scrub",
			actionID:   "pool.scrub",
			req:        connectorsdk.ActionRequest{Params: map[string]string{"pool_name": "mainpool"}},
			wantMethod: "pool.scrub.run", wantStatus: "succeeded", expectCalled: true,
		},
		{
			name:       "snapshot create",
			actionID:   "snapshot.create",
			req:        connectorsdk.ActionRequest{Params: map[string]string{"dataset": "mainpool/data", "name": "snap-1"}},
			wantMethod: "zfs.snapshot.create", wantStatus: "succeeded", expectCalled: true,
		},
		{
			name:       "snapshot delete",
			actionID:   "snapshot.delete",
			req:        connectorsdk.ActionRequest{Params: map[string]string{"snapshot_id": "mainpool/data@snap-1"}},
			wantMethod: "zfs.snapshot.delete", wantStatus: "succeeded", expectCalled: true,
		},
		{
			name:       "service restart",
			actionID:   "service.restart",
			req:        connectorsdk.ActionRequest{Params: map[string]string{"service": "ssh"}},
			wantMethod: "service.restart", wantStatus: "succeeded", expectCalled: true,
		},
		{
			name:       "smart test",
			actionID:   "smart.test",
			req:        connectorsdk.ActionRequest{Params: map[string]string{"disk": "sda", "type": "short"}},
			wantMethod: "smart.test.manual_test", wantStatus: "succeeded", expectCalled: true,
		},
		{
			name:       "vm start",
			actionID:   "vm.start",
			req:        connectorsdk.ActionRequest{Params: map[string]string{"vm_id": "101"}},
			wantMethod: "vm.start", wantStatus: "succeeded", expectCalled: true,
		},
		{
			name:       "vm stop",
			actionID:   "vm.stop",
			req:        connectorsdk.ActionRequest{Params: map[string]string{"vm_id": "101"}},
			wantMethod: "vm.stop", wantStatus: "succeeded", expectCalled: true,
		},
		{
			name:       "system reboot",
			actionID:   "system.reboot",
			req:        connectorsdk.ActionRequest{},
			wantMethod: "system.reboot", wantStatus: "succeeded", expectCalled: true,
		},
		{
			name:       "service start",
			actionID:   "service.start",
			req:        connectorsdk.ActionRequest{Params: map[string]string{"service": "nfs"}},
			wantMethod: "service.start", wantStatus: "succeeded", expectCalled: true,
		},
		{
			name:       "service stop",
			actionID:   "service.stop",
			req:        connectorsdk.ActionRequest{Params: map[string]string{"service": "nfs"}},
			wantMethod: "service.stop", wantStatus: "succeeded", expectCalled: true,
		},
		{
			name:       "app restart",
			actionID:   "app.restart",
			req:        connectorsdk.ActionRequest{Params: map[string]string{"app_name": "portainer"}},
			wantMethod: "app.restart", wantStatus: "succeeded", expectCalled: true,
		},
		{
			name:         "dry run avoids call",
			actionID:     "app.start",
			req:          connectorsdk.ActionRequest{Params: map[string]string{"app_name": "portainer"}, DryRun: true},
			wantStatus:   "succeeded",
			expectCalled: false,
		},
		{
			name:         "unsupported action",
			actionID:     "unknown.action",
			req:          connectorsdk.ActionRequest{},
			wantStatus:   "failed",
			expectCalled: false,
		},
		{
			name:         "invalid vm id",
			actionID:     "vm.start",
			req:          connectorsdk.ActionRequest{Params: map[string]string{"vm_id": "abc"}},
			wantStatus:   "failed",
			expectCalled: false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			gotMethod := ""

			srv := mockTrueNASServer(t, func(conn *websocket.Conn, call rpcCall) {
				calls++
				gotMethod = call.Method
				_ = writeRPCResult(conn, call.ID, map[string]any{"ok": true})
			})
			defer srv.Close()

			connector := &Connector{client: newTestClient(srv.URL)}
			result, err := connector.ExecuteAction(context.Background(), tc.actionID, tc.req)
			if err != nil {
				t.Fatalf("ExecuteAction() error = %v", err)
			}
			if result.Status != tc.wantStatus {
				t.Fatalf("ExecuteAction() status = %q, want %q (message=%q)", result.Status, tc.wantStatus, result.Message)
			}

			if tc.expectCalled {
				if calls != 1 {
					t.Fatalf("expected one RPC call, got %d", calls)
				}
				if gotMethod != tc.wantMethod {
					t.Fatalf("RPC method = %q, want %q", gotMethod, tc.wantMethod)
				}
				return
			}

			if calls != 0 {
				t.Fatalf("expected no RPC calls, got %d (method=%q)", calls, gotMethod)
			}
		})
	}
}

func TestConnectorExecuteActionValidationAndFallbacks(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	connector := NewWithConfig(Config{
		BaseURL: "https://example.invalid",
		APIKey:  "api-key",
	})

	cases := []struct {
		name       string
		actionID   string
		req        connectorsdk.ActionRequest
		wantStatus string
		wantSubstr string
	}{
		{
			name:       "pool scrub requires pool name",
			actionID:   "pool.scrub",
			req:        connectorsdk.ActionRequest{},
			wantStatus: "failed",
			wantSubstr: "pool_name is required",
		},
		{
			name:       "snapshot create requires both fields",
			actionID:   "snapshot.create",
			req:        connectorsdk.ActionRequest{Params: map[string]string{"dataset": "tank/data"}},
			wantStatus: "failed",
			wantSubstr: "dataset and name are required",
		},
		{
			name:       "snapshot delete requires id",
			actionID:   "snapshot.delete",
			req:        connectorsdk.ActionRequest{},
			wantStatus: "failed",
			wantSubstr: "snapshot_id is required",
		},
		{
			name:       "snapshot rollback requires id",
			actionID:   "snapshot.rollback",
			req:        connectorsdk.ActionRequest{},
			wantStatus: "failed",
			wantSubstr: "snapshot_id is required",
		},
		{
			name:       "service restart requires service",
			actionID:   "service.restart",
			req:        connectorsdk.ActionRequest{},
			wantStatus: "failed",
			wantSubstr: "service is required",
		},
		{
			name:       "smart test requires disk",
			actionID:   "smart.test",
			req:        connectorsdk.ActionRequest{},
			wantStatus: "failed",
			wantSubstr: "disk is required",
		},
		{
			name:       "vm start requires id",
			actionID:   "vm.start",
			req:        connectorsdk.ActionRequest{},
			wantStatus: "failed",
			wantSubstr: "vm_id is required",
		},
		{
			name:       "vm stop requires id",
			actionID:   "vm.stop",
			req:        connectorsdk.ActionRequest{},
			wantStatus: "failed",
			wantSubstr: "vm_id is required",
		},
		{
			name:       "vm stop invalid id",
			actionID:   "vm.stop",
			req:        connectorsdk.ActionRequest{Params: map[string]string{"vm_id": "abc"}},
			wantStatus: "failed",
			wantSubstr: "vm_id must be an integer",
		},
		{
			name:       "service start requires service",
			actionID:   "service.start",
			req:        connectorsdk.ActionRequest{},
			wantStatus: "failed",
			wantSubstr: "service is required",
		},
		{
			name:       "service stop requires service",
			actionID:   "service.stop",
			req:        connectorsdk.ActionRequest{},
			wantStatus: "failed",
			wantSubstr: "service is required",
		},
		{
			name:       "app start requires app name",
			actionID:   "app.start",
			req:        connectorsdk.ActionRequest{},
			wantStatus: "failed",
			wantSubstr: "app_name is required",
		},
		{
			name:       "smart test uses target and defaults test type",
			actionID:   "smart.test",
			req:        connectorsdk.ActionRequest{DryRun: true, TargetID: "sdc"},
			wantStatus: "succeeded",
			wantSubstr: "would run SHORT SMART test on disk \"sdc\"",
		},
		{
			name:       "pool scrub uses target fallback",
			actionID:   "pool.scrub",
			req:        connectorsdk.ActionRequest{DryRun: true, TargetID: "tank"},
			wantStatus: "succeeded",
			wantSubstr: "would run pool.scrub.run on pool \"tank\"",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			result, err := connector.ExecuteAction(context.Background(), tc.actionID, tc.req)
			if err != nil {
				t.Fatalf("ExecuteAction() error = %v", err)
			}
			if result.Status != tc.wantStatus {
				t.Fatalf("status = %q, want %q (message=%q)", result.Status, tc.wantStatus, result.Message)
			}
			if tc.wantSubstr != "" {
				combined := result.Message + " " + result.Output
				if !strings.Contains(combined, tc.wantSubstr) {
					t.Fatalf("expected %q to contain %q", combined, tc.wantSubstr)
				}
			}
		})
	}
}

func TestConnectorExecuteActionRPCFailurePaths(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	type testCase struct {
		name       string
		actionID   string
		req        connectorsdk.ActionRequest
		wantMethod string
	}

	cases := []testCase{
		{name: "pool scrub rpc failure", actionID: "pool.scrub", req: connectorsdk.ActionRequest{Params: map[string]string{"pool_name": "tank"}}, wantMethod: "pool.scrub.run"},
		{name: "snapshot create rpc failure", actionID: "snapshot.create", req: connectorsdk.ActionRequest{Params: map[string]string{"dataset": "tank/data", "name": "snap1"}}, wantMethod: "zfs.snapshot.create"},
		{name: "snapshot delete rpc failure", actionID: "snapshot.delete", req: connectorsdk.ActionRequest{Params: map[string]string{"snapshot_id": "tank/data@snap1"}}, wantMethod: "zfs.snapshot.delete"},
		{name: "snapshot rollback rpc failure", actionID: "snapshot.rollback", req: connectorsdk.ActionRequest{Params: map[string]string{"snapshot_id": "tank/data@snap1"}}, wantMethod: "zfs.snapshot.rollback"},
		{name: "service restart rpc failure", actionID: "service.restart", req: connectorsdk.ActionRequest{Params: map[string]string{"service": "ssh"}}, wantMethod: "service.restart"},
		{name: "smart test rpc failure", actionID: "smart.test", req: connectorsdk.ActionRequest{Params: map[string]string{"disk": "sda"}}, wantMethod: "smart.test.manual_test"},
		{name: "vm start rpc failure", actionID: "vm.start", req: connectorsdk.ActionRequest{Params: map[string]string{"vm_id": "101"}}, wantMethod: "vm.start"},
		{name: "vm stop rpc failure", actionID: "vm.stop", req: connectorsdk.ActionRequest{Params: map[string]string{"vm_id": "101"}}, wantMethod: "vm.stop"},
		{name: "system reboot rpc failure", actionID: "system.reboot", req: connectorsdk.ActionRequest{}, wantMethod: "system.reboot"},
		{name: "service start rpc failure", actionID: "service.start", req: connectorsdk.ActionRequest{Params: map[string]string{"service": "nfs"}}, wantMethod: "service.start"},
		{name: "service stop rpc failure", actionID: "service.stop", req: connectorsdk.ActionRequest{Params: map[string]string{"service": "nfs"}}, wantMethod: "service.stop"},
		{name: "app action rpc failure", actionID: "app.stop", req: connectorsdk.ActionRequest{Params: map[string]string{"app_name": "portainer"}}, wantMethod: "app.stop"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			gotMethod := ""
			srv := mockTrueNASServer(t, func(conn *websocket.Conn, call rpcCall) {
				gotMethod = call.Method
				_ = writeRPCError(conn, call.ID, -32000, "operation failed")
			})
			defer srv.Close()

			connector := &Connector{client: newTestClient(srv.URL)}
			result, err := connector.ExecuteAction(context.Background(), tc.actionID, tc.req)
			if err != nil {
				t.Fatalf("ExecuteAction() error = %v", err)
			}
			if result.Status != "failed" {
				t.Fatalf("status = %q, want failed", result.Status)
			}
			if gotMethod != tc.wantMethod {
				t.Fatalf("method = %q, want %q", gotMethod, tc.wantMethod)
			}
		})
	}
}

func TestConnectorExecuteActionSnapshotRollbackSuccess(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	srv := mockTrueNASServer(t, func(conn *websocket.Conn, call rpcCall) {
		_ = writeRPCResult(conn, call.ID, map[string]any{"ok": true})
	})
	defer srv.Close()

	connector := &Connector{client: newTestClient(srv.URL)}
	result, err := connector.ExecuteAction(context.Background(), "snapshot.rollback", connectorsdk.ActionRequest{
		Params: map[string]string{"snapshot_id": "tank/data@snap1"},
	})
	if err != nil {
		t.Fatalf("ExecuteAction() error = %v", err)
	}
	if result.Status != "succeeded" {
		t.Fatalf("status = %q, want succeeded", result.Status)
	}
}

func TestConnectorExecuteActionDryRunCoverage(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	connector := NewWithConfig(Config{
		BaseURL: "https://example.invalid",
		APIKey:  "api-key",
	})

	tests := []struct {
		name     string
		actionID string
		req      connectorsdk.ActionRequest
	}{
		{name: "pool scrub", actionID: "pool.scrub", req: connectorsdk.ActionRequest{DryRun: true, Params: map[string]string{"pool_name": "mainpool"}}},
		{name: "snapshot create", actionID: "snapshot.create", req: connectorsdk.ActionRequest{DryRun: true, Params: map[string]string{"dataset": "mainpool/data", "name": "snap-1"}}},
		{name: "snapshot delete", actionID: "snapshot.delete", req: connectorsdk.ActionRequest{DryRun: true, Params: map[string]string{"snapshot_id": "mainpool/data@snap-1"}}},
		{name: "snapshot rollback", actionID: "snapshot.rollback", req: connectorsdk.ActionRequest{DryRun: true, Params: map[string]string{"snapshot_id": "mainpool/data@snap-1"}}},
		{name: "service restart", actionID: "service.restart", req: connectorsdk.ActionRequest{DryRun: true, Params: map[string]string{"service": "ssh"}}},
		{name: "smart test", actionID: "smart.test", req: connectorsdk.ActionRequest{DryRun: true, Params: map[string]string{"disk": "sda", "type": "short"}}},
		{name: "vm start", actionID: "vm.start", req: connectorsdk.ActionRequest{DryRun: true, Params: map[string]string{"vm_id": "101"}}},
		{name: "vm stop", actionID: "vm.stop", req: connectorsdk.ActionRequest{DryRun: true, Params: map[string]string{"vm_id": "101"}}},
		{name: "system reboot", actionID: "system.reboot", req: connectorsdk.ActionRequest{DryRun: true}},
		{name: "service start", actionID: "service.start", req: connectorsdk.ActionRequest{DryRun: true, Params: map[string]string{"service": "nfs"}}},
		{name: "service stop", actionID: "service.stop", req: connectorsdk.ActionRequest{DryRun: true, Params: map[string]string{"service": "nfs"}}},
		{name: "app start", actionID: "app.start", req: connectorsdk.ActionRequest{DryRun: true, Params: map[string]string{"app_name": "portainer"}}},
		{name: "app stop", actionID: "app.stop", req: connectorsdk.ActionRequest{DryRun: true, Params: map[string]string{"app_name": "portainer"}}},
		{name: "app restart", actionID: "app.restart", req: connectorsdk.ActionRequest{DryRun: true, Params: map[string]string{"app_name": "portainer"}}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			result, err := connector.ExecuteAction(context.Background(), tt.actionID, tt.req)
			if err != nil {
				t.Fatalf("ExecuteAction() error = %v", err)
			}
			if result.Status != "succeeded" {
				t.Fatalf("ExecuteAction() status = %q, want succeeded (message=%q)", result.Status, result.Message)
			}
		})
	}
}

package truenas

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/connectorsdk"
)

func TestConnectorDiscoverHappyPath(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	srv := mockTrueNASServer(t, func(conn *websocket.Conn, call rpcCall) {
		switch call.Method {
		case "system.info":
			_ = writeRPCResult(conn, call.ID, map[string]any{
				"hostname": "OmegaNAS",
				"version":  "25.04.0",
				"model":    "Mini",
				"cores":    8,
				"physmem":  34359738368,
				"uptime":   "1 day",
				"loadavg":  []any{1.5, 0.8, 0.4},
			})
		case "pool.query":
			_ = writeRPCResult(conn, call.ID, []map[string]any{
				{
					"id":            1,
					"name":          "mainpool",
					"status":        "ONLINE",
					"healthy":       true,
					"size":          1000.0,
					"allocated":     250.0,
					"free":          750.0,
					"fragmentation": "3",
					"scan": map[string]any{
						"state":  "FINISHED",
						"errors": 0,
					},
				},
			})
		case "pool.dataset.query":
			_ = writeRPCResult(conn, call.ID, []map[string]any{
				{
					"name":        "mainpool/data",
					"mountpoint":  map[string]any{"value": "/mnt/mainpool/data"},
					"used":        map[string]any{"rawvalue": "128"},
					"available":   map[string]any{"rawvalue": "872"},
					"quota":       map[string]any{"rawvalue": "0"},
					"readonly":    map[string]any{"parsed": false},
					"compression": map[string]any{"value": "lz4"},
				},
			})
		case "disk.query":
			_ = writeRPCResult(conn, call.ID, []map[string]any{
				{
					"name":   "sda",
					"serial": "XYZ123",
					"size":   500.0,
					"model":  "SSD",
					"type":   "SSD",
				},
			})
		case "disk.temperatures":
			_ = writeRPCResult(conn, call.ID, map[string]any{"sda": 39})
		case "sharing.smb.query":
			_ = writeRPCResult(conn, call.ID, []map[string]any{
				{"name": "shared", "path": "/mnt/mainpool/shared", "enabled": true},
			})
		case "sharing.nfs.query":
			_ = writeRPCResult(conn, call.ID, []map[string]any{
				{"id": 11, "path": "/mnt/mainpool/nfs", "enabled": true},
			})
		case "service.query":
			_ = writeRPCResult(conn, call.ID, []map[string]any{
				{"service": "ssh", "state": "RUNNING", "enable": true},
			})
		case "vm.query":
			_ = writeRPCResult(conn, call.ID, []map[string]any{
				{"id": 101, "name": "truenas-vm", "status": map[string]any{"state": "RUNNING"}, "vcpus": 2, "memory": 2048},
			})
		case "app.query":
			_ = writeRPCResult(conn, call.ID, []map[string]any{
				{"name": "portainer", "state": "RUNNING", "version": "1.0.0"},
			})
		default:
			_ = writeRPCError(conn, call.ID, -32601, "Method not found")
		}
	})
	defer srv.Close()

	connector := &Connector{client: newTestClient(srv.URL)}
	assets, err := connector.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(assets) < 8 {
		t.Fatalf("Discover() returned %d assets, want at least 8", len(assets))
	}
	assertHasAssetType(t, assets, "nas")
	assertHasAssetType(t, assets, "storage-pool")
	assertHasAssetType(t, assets, "dataset")
	assertHasAssetType(t, assets, "disk")
	assertHasAssetType(t, assets, "share-smb")
	assertHasAssetType(t, assets, "share-nfs")
	assertHasAssetType(t, assets, "service")
	assertHasAssetType(t, assets, "vm")
	assertHasAssetType(t, assets, "app")
}

func TestConnectorDiscoverPoolQueryFailureIsFatal(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	srv := mockTrueNASServer(t, func(conn *websocket.Conn, call rpcCall) {
		switch call.Method {
		case "system.info":
			_ = writeRPCResult(conn, call.ID, map[string]any{"hostname": "OmegaNAS"})
		case "pool.query":
			_ = writeRPCError(conn, call.ID, -32000, "permission denied")
		default:
			_ = writeRPCResult(conn, call.ID, []map[string]any{})
		}
	})
	defer srv.Close()

	connector := &Connector{client: newTestClient(srv.URL)}
	_, err := connector.Discover(context.Background())
	if err == nil {
		t.Fatalf("expected Discover() to fail when pool.query fails")
	}
}

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

func TestConnectorNotConfiguredPaths(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	connector := New()

	health, err := connector.TestConnection(context.Background())
	if err != nil {
		t.Fatalf("TestConnection() unexpected error: %v", err)
	}
	if health.Status != "ok" {
		t.Fatalf("TestConnection().Status = %q, want ok", health.Status)
	}

	assets, err := connector.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover() unexpected error in stub mode: %v", err)
	}
	if len(assets) == 0 {
		t.Fatalf("expected stub asset in unconfigured mode")
	}

	result, err := connector.ExecuteAction(context.Background(), "system.reboot", connectorsdk.ActionRequest{})
	if err != nil {
		t.Fatalf("ExecuteAction() unexpected error: %v", err)
	}
	if result.Status != "failed" {
		t.Fatalf("ExecuteAction() status = %q, want failed", result.Status)
	}
}

func TestConnectorConfiguredMetadataAndHealth(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	srv := mockTrueNASServer(t, func(conn *websocket.Conn, call rpcCall) {
		if call.Method != "system.info" {
			_ = writeRPCError(conn, call.ID, -32601, "Method not found")
			return
		}
		_ = writeRPCResult(conn, call.ID, map[string]any{
			"hostname": "OmegaNAS",
			"version":  "25.04.0",
		})
	})
	defer srv.Close()

	connector := NewWithConfig(Config{
		BaseURL: serverURLToWS(srv.URL),
		APIKey:  "test-api-key",
	})
	if connector.ID() != "truenas" {
		t.Fatalf("ID() = %q, want truenas", connector.ID())
	}
	if connector.DisplayName() != "TrueNAS" {
		t.Fatalf("DisplayName() = %q, want TrueNAS", connector.DisplayName())
	}
	caps := connector.Capabilities()
	if !caps.DiscoverAssets || !caps.CollectMetrics || !caps.CollectEvents || !caps.ExecuteActions {
		t.Fatalf("unexpected connector capabilities: %+v", caps)
	}
	actions := connector.Actions()
	if len(actions) == 0 {
		t.Fatalf("expected actions to be populated")
	}

	health, err := connector.TestConnection(context.Background())
	if err != nil {
		t.Fatalf("TestConnection() error = %v", err)
	}
	if health.Status != "ok" {
		t.Fatalf("TestConnection().Status = %q, want ok", health.Status)
	}
}

func TestConnectorTestConnectionFailureAndMessageVariants(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	t.Run("rpc failure returns failed health", func(t *testing.T) {
		srv := mockTrueNASServer(t, func(conn *websocket.Conn, call rpcCall) {
			_ = writeRPCError(conn, call.ID, -32000, "permission denied")
		})
		defer srv.Close()

		connector := &Connector{client: newTestClient(srv.URL)}
		health, err := connector.TestConnection(context.Background())
		if err != nil {
			t.Fatalf("TestConnection() error = %v", err)
		}
		if health.Status != "failed" {
			t.Fatalf("status = %q, want failed", health.Status)
		}
	})

	cases := []struct {
		name    string
		payload map[string]any
		wantMsg string
	}{
		{
			name:    "hostname only",
			payload: map[string]any{"hostname": "OmegaNAS"},
			wantMsg: "connected to OmegaNAS",
		},
		{
			name:    "version only",
			payload: map[string]any{"version": "25.04.0"},
			wantMsg: "truenas reachable, version 25.04.0",
		},
		{
			name:    "neither hostname nor version",
			payload: map[string]any{},
			wantMsg: "truenas reachable",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			srv := mockTrueNASServer(t, func(conn *websocket.Conn, call rpcCall) {
				if call.Method != "system.info" {
					_ = writeRPCError(conn, call.ID, -32601, "Method not found")
					return
				}
				_ = writeRPCResult(conn, call.ID, tc.payload)
			})
			defer srv.Close()

			connector := &Connector{client: newTestClient(srv.URL)}
			health, err := connector.TestConnection(context.Background())
			if err != nil {
				t.Fatalf("TestConnection() error = %v", err)
			}
			if health.Status != "ok" {
				t.Fatalf("status = %q, want ok", health.Status)
			}
			if health.Message != tc.wantMsg {
				t.Fatalf("message = %q, want %q", health.Message, tc.wantMsg)
			}
		})
	}
}

func TestConnectorDiscoverReturnsStubWhenOptionalQueriesAllFail(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	srv := mockTrueNASServer(t, func(conn *websocket.Conn, call rpcCall) {
		switch call.Method {
		case "system.info":
			_ = writeRPCError(conn, call.ID, -32000, "failed")
		case "pool.query":
			_ = writeRPCResult(conn, call.ID, []map[string]any{})
		case "pool.dataset.query", "disk.query", "sharing.smb.query", "sharing.nfs.query", "service.query":
			_ = writeRPCError(conn, call.ID, -32000, "permission denied")
		case "vm.query", "app.query":
			_ = writeRPCError(conn, call.ID, -32000, "runtime unavailable")
		default:
			_ = writeRPCError(conn, call.ID, -32601, "Method not found")
		}
	})
	defer srv.Close()

	connector := &Connector{client: newTestClient(srv.URL)}
	assets, err := connector.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(assets) != 1 || assets[0].ID != "truenas-controller-stub" {
		t.Fatalf("Discover() expected stub fallback asset, got %#v", assets)
	}
}

func TestConnectorDiscoverSkipsScaleOnlyMethodsOnCORE(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	srv := mockTrueNASServer(t, func(conn *websocket.Conn, call rpcCall) {
		switch call.Method {
		case "system.info":
			_ = writeRPCResult(conn, call.ID, map[string]any{"hostname": "core-box"})
		case "pool.query":
			_ = writeRPCResult(conn, call.ID, []map[string]any{{"id": 1, "name": "tank"}})
		case "pool.dataset.query", "disk.query", "sharing.smb.query", "sharing.nfs.query", "service.query":
			_ = writeRPCResult(conn, call.ID, []map[string]any{})
		case "vm.query", "app.query":
			_ = writeRPCError(conn, call.ID, -32601, "Method not found")
		default:
			_ = writeRPCError(conn, call.ID, -32601, "Method not found")
		}
	})
	defer srv.Close()

	connector := &Connector{client: newTestClient(srv.URL)}
	assets, err := connector.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	assertHasAssetType(t, assets, "nas")
	assertHasAssetType(t, assets, "storage-pool")
	for _, asset := range assets {
		if asset.Type == "vm" || asset.Type == "app" {
			t.Fatalf("expected vm/app assets to be skipped for CORE methods, got %#v", asset)
		}
	}
}

func TestConnectorDiscoverFallbacksAndFiltering(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	srv := mockTrueNASServer(t, func(conn *websocket.Conn, call rpcCall) {
		switch call.Method {
		case "system.info":
			_ = writeRPCResult(conn, call.ID, map[string]any{
				"hostname": "",
				"version":  "25.04.0",
				"cores":    0,
				"loadavg":  []any{2.0},
			})
		case "pool.query":
			_ = writeRPCResult(conn, call.ID, []map[string]any{
				{},
				{
					"id":        7,
					"name":      "",
					"size":      0,
					"allocated": 0,
					"free":      0,
				},
			})
		case "pool.dataset.query":
			_ = writeRPCResult(conn, call.ID, []map[string]any{
				{"name": ""},
				{"name": "tank/data", "mountpoint": map[string]any{"rawvalue": "/mnt/tank/data"}},
			})
		case "disk.query":
			_ = writeRPCResult(conn, call.ID, []map[string]any{
				{"name": ""},
				{"name": "sdb", "serial": "SER1", "size": "123"},
			})
		case "disk.temperatures":
			_ = writeRPCError(conn, call.ID, -32000, "temps unavailable")
		case "sharing.smb.query":
			_ = writeRPCResult(conn, call.ID, []map[string]any{
				{"id": "share-id", "name": "", "path": "/mnt/tank/share"},
				{"id": "", "name": ""},
			})
		case "sharing.nfs.query":
			_ = writeRPCResult(conn, call.ID, []map[string]any{
				{"id": 42, "path": ""},
				{"id": "", "path": ""},
			})
		case "service.query":
			_ = writeRPCResult(conn, call.ID, []map[string]any{
				{"service": "", "state": "RUNNING"},
				{"service": "ssh", "state": "RUNNING", "enable": true},
			})
		case "vm.query":
			_ = writeRPCResult(conn, call.ID, []map[string]any{
				{"id": 200, "name": "", "status": "RUNNING"},
				{"id": "", "name": "", "status": "STOPPED"},
			})
		case "app.query":
			_ = writeRPCResult(conn, call.ID, []map[string]any{
				{"name": ""},
				{"name": "plex", "state": "RUNNING", "version": "1.0.0"},
			})
		default:
			_ = writeRPCError(conn, call.ID, -32601, "Method not found")
		}
	})
	defer srv.Close()

	connector := &Connector{client: newTestClient(srv.URL)}
	assets, err := connector.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	host := findAssetByTypeAndName(assets, "nas", "truenas")
	if host == nil {
		t.Fatalf("expected fallback truenas host asset in %#v", assets)
	}
	pool := findAssetByTypeAndName(assets, "storage-pool", "pool-7")
	if pool == nil {
		t.Fatalf("expected fallback pool asset in %#v", assets)
	}
	if _, ok := pool.Metadata["disk_used_percent"]; ok {
		t.Fatalf("did not expect disk_used_percent for zero-sized pool: %#v", pool.Metadata)
	}
	if findAssetByTypeAndName(assets, "dataset", "tank/data") == nil {
		t.Fatalf("expected dataset asset in %#v", assets)
	}
	if findAssetByTypeAndName(assets, "disk", "sdb") == nil {
		t.Fatalf("expected disk asset in %#v", assets)
	}
	if findAssetByTypeAndName(assets, "share-smb", "share-id") == nil {
		t.Fatalf("expected smb fallback-id asset in %#v", assets)
	}
	if findAssetByTypeAndName(assets, "share-nfs", "nfs-42") == nil {
		t.Fatalf("expected nfs fallback-id asset in %#v", assets)
	}
	if findAssetByTypeAndName(assets, "vm", "vm-200") == nil {
		t.Fatalf("expected vm fallback-id asset in %#v", assets)
	}
	if findAssetByTypeAndName(assets, "app", "plex") == nil {
		t.Fatalf("expected app asset in %#v", assets)
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

func assertHasAssetType(t *testing.T, assets []connectorsdk.Asset, assetType string) {
	t.Helper()
	for _, asset := range assets {
		if asset.Type == assetType {
			return
		}
	}
	encoded, _ := json.Marshal(assets)
	t.Fatalf("expected asset type %q in discover results: %s", assetType, string(encoded))
}

func findAssetByTypeAndName(assets []connectorsdk.Asset, assetType, name string) *connectorsdk.Asset {
	for _, asset := range assets {
		if asset.Type == assetType && asset.Name == name {
			match := asset
			return &match
		}
	}
	return nil
}

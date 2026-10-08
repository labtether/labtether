package truenas

import (
	"context"
	"encoding/json"
	"github.com/gorilla/websocket"
	"github.com/labtether/labtether/internal/connectorsdk"
	"strings"
	"testing"
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

func TestConnectorUnconfiguredModeFailsClosed(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	connector := New()

	health, err := connector.TestConnection(context.Background())
	if err != nil {
		t.Fatalf("TestConnection() unexpected error: %v", err)
	}
	if health.Status != "failed" || !strings.Contains(health.Message, "not configured") {
		t.Fatalf("TestConnection() = %+v, want failed unconfigured health", health)
	}

	assets, err := connector.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover() unexpected error in unconfigured mode: %v", err)
	}
	if assets == nil || len(assets) != 0 {
		t.Fatalf("expected non-nil empty unconfigured inventory, got %+v", assets)
	}

	for _, descriptor := range connector.Actions() {
		for _, dryRun := range []bool{false, true} {
			result, execErr := connector.ExecuteAction(context.Background(), descriptor.ID, connectorsdk.ActionRequest{DryRun: dryRun})
			if execErr != nil || result.Status != "failed" || !strings.Contains(result.Message, "not configured") {
				t.Fatalf("ExecuteAction(%q, dry_run=%v) = %+v, err=%v, want fail-closed unconfigured result", descriptor.ID, dryRun, result, execErr)
			}
		}
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

func TestConnectorDiscoverReturnsNoSyntheticInventoryWhenOptionalQueriesAllFail(t *testing.T) {
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
	if assets == nil || len(assets) != 0 {
		t.Fatalf("Discover() expected non-nil empty inventory, got %#v", assets)
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

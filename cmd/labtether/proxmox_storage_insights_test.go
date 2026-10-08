package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/connectors/proxmox"
	proxmoxpkg "github.com/labtether/labtether/internal/hubapi/proxmox"
	"github.com/labtether/labtether/internal/telemetry"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLoadProxmoxStorageInsights(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)

	sut := newTestAPIServer(t)

	_, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "proxmox-node-pve01",
		Type:    "hypervisor-node",
		Name:    "pve01",
		Source:  "proxmox",
		Status:  "online",
		Metadata: map[string]string{
			"proxmox_type": "node",
			"node":         "pve01",
		},
	})
	if err != nil {
		t.Fatalf("failed to seed host asset: %v", err)
	}
	_, err = sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "proxmox-storage-local-zfs",
		Type:    "storage-pool",
		Name:    "storage/pve01/local-zfs",
		Source:  "proxmox",
		Status:  "online",
		Metadata: map[string]string{
			"proxmox_type": "storage",
			"node":         "pve01",
			"storage_id":   "storage/pve01/local-zfs",
			"disk_percent": "84",
		},
	})
	if err != nil {
		t.Fatalf("failed to seed storage asset: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	samples := make([]telemetry.MetricSample, 0, 7)
	for idx := 0; idx < 7; idx++ {
		samples = append(samples, telemetry.MetricSample{
			AssetID:     "proxmox-storage-local-zfs",
			Metric:      telemetry.MetricDiskUsedPercent,
			Unit:        "percent",
			Value:       72 + float64(idx*2),
			CollectedAt: now.Add(-time.Duration(6-idx) * 24 * time.Hour),
		})
	}
	if err := sut.telemetryStore.AppendSamples(context.Background(), samples); err != nil {
		t.Fatalf("failed to seed telemetry: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/nodes/pve01/disks/zfs":
			_, _ = w.Write([]byte(`{"data":[{"name":"local-zfs","size":1000,"alloc":840,"free":160,"frag":12,"health":"ONLINE","dedup":1.04}]}`))
		case "/api2/json/nodes/pve01/storage/local-zfs/content":
			_, _ = w.Write([]byte(`{"data":[{"volid":"local-zfs:backup/vzdump-qemu-101.vma.zst","content":"backup","size":200,"vmid":101},{"volid":"local-zfs:subvol-201-disk-0","content":"rootdir","size":80,"vmid":201}]}`))
		case "/api2/json/nodes/pve01/tasks":
			_, _ = w.Write([]byte(fmt.Sprintf(`{"data":[{"upid":"UPID:pve01:001:001:001:vzdump:101:root@pam:","node":"pve01","id":"101","type":"vzdump","status":"stopped","exitstatus":"OK","starttime":%d}]}`, now.Unix())))
		default:
			t.Fatalf("unexpected proxmox path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := proxmox.NewClient(proxmox.Config{
		BaseURL:     server.URL,
		TokenID:     "id",
		TokenSecret: "secret",
		Timeout:     5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	runtime := proxmoxpkg.NewProxmoxRuntimeWithCollector(client, "collector-1")
	target := proxmoxSessionTarget{
		Kind: "node",
		Node: "pve01",
	}

	resp, err := sut.loadProxmoxStorageInsights(context.Background(), "proxmox-node-pve01", target, runtime, 7*24*time.Hour)
	if err != nil {
		t.Fatalf("loadProxmoxStorageInsights failed: %v", err)
	}

	if resp.Window != "7d" {
		t.Fatalf("expected window 7d, got %q", resp.Window)
	}
	if len(resp.Pools) != 1 {
		t.Fatalf("expected 1 pool, got %d", len(resp.Pools))
	}

	pool := resp.Pools[0]
	if pool.Name != "local-zfs" {
		t.Fatalf("unexpected pool name: %q", pool.Name)
	}
	if pool.UsedPercent == nil || *pool.UsedPercent < 80 {
		t.Fatalf("expected high used percent, got %#v", pool.UsedPercent)
	}
	if pool.Forecast.DaysToFull == nil {
		t.Fatalf("expected forecast days_to_full")
	}
	if pool.GrowthBytes7D == nil || *pool.GrowthBytes7D <= 0 {
		t.Fatalf("expected positive growth bytes, got %#v", pool.GrowthBytes7D)
	}
	if pool.DependentWorkloads.VMCount != 1 || pool.DependentWorkloads.CTCount != 1 {
		t.Fatalf("unexpected dependent workloads: %+v", pool.DependentWorkloads)
	}
	if len(pool.DependentWorkloads.VMIDs) != 1 || pool.DependentWorkloads.VMIDs[0] != 101 {
		t.Fatalf("unexpected vm id list: %+v", pool.DependentWorkloads.VMIDs)
	}
	if len(pool.DependentWorkloads.CTIDs) != 1 || pool.DependentWorkloads.CTIDs[0] != 201 {
		t.Fatalf("unexpected ct id list: %+v", pool.DependentWorkloads.CTIDs)
	}
	if pool.Snapshots.Count != 1 || pool.Snapshots.Bytes != 200 {
		t.Fatalf("unexpected snapshot summary: %+v", pool.Snapshots)
	}
	if resp.Summary.PredictedFullLT30D != 1 {
		t.Fatalf("expected predicted_full_lt_30d=1, got %d", resp.Summary.PredictedFullLT30D)
	}
	if len(resp.Events) == 0 {
		t.Fatalf("expected non-empty storage timeline events")
	}
	if resp.Events[0].Pool != "local-zfs" {
		t.Fatalf("expected event mapped to local-zfs, got %+v", resp.Events[0])
	}
	if resp.Events[0].UPID == "" || resp.Events[0].Node != "pve01" {
		t.Fatalf("expected event task metadata, got %+v", resp.Events[0])
	}
}

func TestLoadProxmoxStorageInsightsMissingNodeAndWarningAggregation(t *testing.T) {
	sut := newTestAPIServer(t)

	_, err := sut.loadProxmoxStorageInsights(
		context.Background(),
		"asset-1",
		proxmoxSessionTarget{Kind: "node"},
		proxmoxpkg.NewProxmoxRuntime(nil),
		7*24*time.Hour,
	)
	if !errors.Is(err, proxmoxpkg.ErrProxmoxMissingNode) {
		t.Fatalf("expected proxmoxpkg.ErrProxmoxMissingNode, got %v", err)
	}

	_, err = sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "proxmox-storage-local-zfs",
		Type:    "storage-pool",
		Name:    "storage/pve01/local-zfs",
		Source:  "proxmox",
		Status:  "online",
		Metadata: map[string]string{
			"proxmox_type": "storage",
			"node":         "pve01",
			"storage_id":   "storage/pve01/local-zfs",
		},
	})
	if err != nil {
		t.Fatalf("failed to seed storage asset: %v", err)
	}

	errorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"errors":{"upstream":"failed"}}`))
	}))
	defer errorServer.Close()

	client, err := proxmox.NewClient(proxmox.Config{
		BaseURL:     errorServer.URL,
		TokenID:     "id",
		TokenSecret: "secret",
		Timeout:     5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	resp, err := sut.loadProxmoxStorageInsights(
		context.Background(),
		"proxmox-node-pve01",
		proxmoxSessionTarget{Kind: "node", Node: "pve01"},
		proxmoxpkg.NewProxmoxRuntimeWithCollector(client, "collector-1"),
		7*24*time.Hour,
	)
	if err != nil {
		t.Fatalf("did not expect loadProxmoxStorageInsights to fail when sources are degraded: %v", err)
	}
	if len(resp.Warnings) < 3 {
		t.Fatalf("expected aggregated warnings for zfs/status/content/tasks failures, got %+v", resp.Warnings)
	}
	if len(resp.Pools) != 1 || resp.Pools[0].Name != "local-zfs" {
		t.Fatalf("expected pool state from assets even with upstream failures, got %+v", resp.Pools)
	}
}

func TestLoadProxmoxStorageInsightsStorageTargetFallbackState(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)

	sut := newTestAPIServer(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/nodes/pve01/disks/zfs":
			_, _ = w.Write([]byte(`{"data":[{"name":"scratch","size":1000,"alloc":200,"free":800,"health":"ONLINE"}]}`))
		case "/api2/json/nodes/pve01/storage/scratch/content":
			_, _ = w.Write([]byte(`{"data":[]}`))
		case "/api2/json/nodes/pve01/tasks":
			_, _ = w.Write([]byte(`{"data":[]}`))
		default:
			t.Fatalf("unexpected proxmox path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := proxmox.NewClient(proxmox.Config{
		BaseURL:     server.URL,
		TokenID:     "id",
		TokenSecret: "secret",
		Timeout:     5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	resp, err := sut.loadProxmoxStorageInsights(
		context.Background(),
		"missing-storage-asset",
		proxmoxSessionTarget{Kind: "storage", Node: "pve01", StorageName: "scratch"},
		proxmoxpkg.NewProxmoxRuntimeWithCollector(client, "collector-1"),
		24*time.Hour,
	)
	if err != nil {
		t.Fatalf("loadProxmoxStorageInsights storage fallback failed: %v", err)
	}
	if len(resp.Pools) != 1 || resp.Pools[0].Name != "scratch" {
		t.Fatalf("expected storage fallback pool named scratch, got %+v", resp.Pools)
	}
	if resp.Pools[0].UsedPercent == nil || *resp.Pools[0].UsedPercent <= 0 {
		t.Fatalf("expected used percent from ZFS data, got %+v", resp.Pools[0])
	}
}

func TestLoadProxmoxStorageInsightsEmptyState(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)

	sut := newTestAPIServer(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/nodes/pve01/disks/zfs":
			_, _ = w.Write([]byte(`{"data":[{"name":""}]}`))
		case "/api2/json/nodes/pve01/tasks":
			_, _ = w.Write([]byte(`{"data":[]}`))
		default:
			t.Fatalf("unexpected proxmox path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := proxmox.NewClient(proxmox.Config{
		BaseURL:     server.URL,
		TokenID:     "id",
		TokenSecret: "secret",
		Timeout:     5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	resp, err := sut.loadProxmoxStorageInsights(
		context.Background(),
		"proxmox-node-pve01",
		proxmoxSessionTarget{Kind: "node", Node: "pve01"},
		proxmoxpkg.NewProxmoxRuntimeWithCollector(client, "collector-1"),
		24*time.Hour,
	)
	if err != nil {
		t.Fatalf("loadProxmoxStorageInsights empty-state failed: %v", err)
	}
	if resp.Pools != nil || resp.Events != nil || resp.Warnings != nil {
		t.Fatalf("expected nil slices in empty-state response, got pools=%+v events=%+v warnings=%+v", resp.Pools, resp.Events, resp.Warnings)
	}
}

func TestLoadProxmoxStorageInsightsAdditionalBranches(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)

	sut := newTestAPIServer(t)

	_, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "proxmox-storage-alpha",
		Type:    "storage-pool",
		Name:    "storage/pve01/alpha",
		Source:  "proxmox",
		Status:  "online",
		Metadata: map[string]string{
			"proxmox_type":  "storage",
			"node":          "pve01",
			"storage_id":    "storage/pve01/alpha",
			"scrub_overdue": "true",
		},
	})
	if err != nil {
		t.Fatalf("failed to seed alpha storage asset: %v", err)
	}
	sut.telemetryStore = &proxmoxTelemetryStoreWithSeriesError{
		inner:       sut.telemetryStore,
		failingID:   "proxmox-storage-alpha",
		seriesError: errors.New("telemetry offline"),
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/nodes/pve01/disks/zfs":
			_, _ = w.Write([]byte(`{"data":[{"name":"alpha","size":1000,"alloc":500,"free":500,"health":"ONLINE"},{"name":"beta","size":1000,"alloc":500,"free":500,"health":"ONLINE"},{"name":"gamma","size":1000,"alloc":600,"free":400,"health":"ONLINE"}]}`))
		case "/api2/json/nodes/pve01/storage/alpha/content",
			"/api2/json/nodes/pve01/storage/beta/content",
			"/api2/json/nodes/pve01/storage/gamma/content":
			_, _ = w.Write([]byte(`{"data":[]}`))
		case "/api2/json/nodes/pve01/tasks":
			_, _ = w.Write([]byte(`{"data":[]}`))
		default:
			t.Fatalf("unexpected proxmox path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := proxmox.NewClient(proxmox.Config{
		BaseURL:     server.URL,
		TokenID:     "id",
		TokenSecret: "secret",
		Timeout:     5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	resp, err := sut.loadProxmoxStorageInsights(
		context.Background(),
		"proxmox-node-pve01",
		proxmoxSessionTarget{Kind: "node", Node: "pve01"},
		proxmoxpkg.NewProxmoxRuntimeWithCollector(client, "collector-1"),
		24*time.Hour,
	)
	if err != nil {
		t.Fatalf("loadProxmoxStorageInsights failed: %v", err)
	}
	if len(resp.Pools) != 3 {
		t.Fatalf("expected 3 pools after ZFS-only append, got %d (%+v)", len(resp.Pools), resp.Pools)
	}
	if resp.Pools[0].Name != "gamma" || resp.Pools[1].Name != "alpha" || resp.Pools[2].Name != "beta" {
		t.Fatalf("expected risk/usage/name sort order gamma->alpha->beta, got %+v", resp.Pools)
	}
	if !strings.Contains(strings.Join(resp.Warnings, " | "), "telemetry series unavailable for alpha") {
		t.Fatalf("expected telemetry warning for alpha, got %+v", resp.Warnings)
	}
	if resp.Summary.ScrubOverdue != 1 {
		t.Fatalf("expected scrub_overdue summary increment, got %d", resp.Summary.ScrubOverdue)
	}
}

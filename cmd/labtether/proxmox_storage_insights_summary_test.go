package main

import (
	"context"
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

func TestBuildProxmoxStorageInsightEventsFiltersAndMapsPools(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	states := []proxmoxStoragePoolState{
		{
			PoolName: "local-zfs",
			Content: []proxmox.StorageContent{
				{
					VolID:   "local-zfs:vm-101-disk-0",
					Content: "images",
					VMID:    101,
				},
			},
		},
	}

	tasks := []proxmox.Task{
		{
			UPID:       "UPID:pve01:001:001:001:vzdump:101:root@pam:",
			Node:       "pve01",
			ID:         "101",
			Type:       "vzdump",
			Status:     "stopped",
			ExitStatus: "OK",
			StartTime:  float64(now.Add(-2 * time.Hour).Unix()),
		},
		{
			UPID:       "UPID:pve01:001:001:001:qmstart:101:root@pam:",
			Node:       "pve01",
			ID:         "101",
			Type:       "qmstart",
			Status:     "stopped",
			ExitStatus: "OK",
			StartTime:  float64(now.Add(-1 * time.Hour).Unix()),
		},
		{
			UPID:       "UPID:pve01:001:001:001:vzdump:101:root@pam:",
			Node:       "pve01",
			ID:         "101",
			Type:       "vzdump",
			Status:     "stopped",
			ExitStatus: "OK",
			StartTime:  float64(now.Add(-30 * time.Hour).Unix()),
		},
	}

	events := buildProxmoxStorageInsightEvents(tasks, states, now, 24*time.Hour)
	if len(events) != 1 {
		t.Fatalf("expected exactly one storage event, got %d", len(events))
	}
	if events[0].Pool != "local-zfs" {
		t.Fatalf("expected event pool local-zfs, got %+v", events[0])
	}
	if events[0].TaskType != "vzdump" || events[0].UPID == "" {
		t.Fatalf("expected mapped vzdump task event, got %+v", events[0])
	}
}

func TestHandleProxmoxAssetsStorageInsightsSuccessAndErrors(t *testing.T) {
	sut := newTestAPIServer(t)

	_, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "proxmox-storage-bad",
		Type:    "storage-pool",
		Name:    "broken",
		Source:  "proxmox",
		Status:  "online",
		Metadata: map[string]string{
			"proxmox_type": "storage",
			"storage_id":   "local",
		},
	})
	if err != nil {
		t.Fatalf("failed to seed broken storage asset: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/proxmox/assets/proxmox-storage-bad/storage/insights", nil)
	rec := httptest.NewRecorder()
	sut.handleProxmoxAssets(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 when proxmox asset metadata is incomplete, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/proxmox/assets/proxmox-storage-bad/storage/other", nil)
	rec = httptest.NewRecorder()
	sut.handleProxmoxAssets(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown storage sub-action, got %d", rec.Code)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/nodes/pve01/disks/zfs":
			_, _ = w.Write([]byte(`{"data":[]}`))
		case "/api2/json/nodes/pve01/storage/local-zfs/status":
			_, _ = w.Write([]byte(`{"data":{"total":1000,"used":500,"avail":500}}`))
		case "/api2/json/nodes/pve01/storage/local-zfs/content":
			_, _ = w.Write([]byte(`{"data":[]}`))
		case "/api2/json/nodes/pve01/tasks":
			_, _ = w.Write([]byte(`{"data":[]}`))
		default:
			t.Fatalf("unexpected proxmox path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	configureSingleProxmoxCollector(t, sut, server.URL, "collector-proxmox-1")

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
			"collector_id": "collector-proxmox-1",
		},
	})
	if err != nil {
		t.Fatalf("failed to seed storage asset: %v", err)
	}

	req = httptest.NewRequest(http.MethodGet, "/proxmox/assets/proxmox-storage-local-zfs/storage/insights?window=36h", nil)
	rec = httptest.NewRecorder()
	sut.handleProxmoxAssets(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for storage insights handler, got %d body=%s", rec.Code, strings.TrimSpace(rec.Body.String()))
	}
	if !strings.Contains(rec.Body.String(), `"window":"36h0m0s"`) {
		t.Fatalf("expected storage insights response window payload, got %s", rec.Body.String())
	}
}

func TestLoadProxmoxStorageInsightsSummaryAndSorting(t *testing.T) {
	sut := newTestAPIServer(t)

	_, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "proxmox-storage-pool-a",
		Type:    "storage-pool",
		Name:    "storage/pve01/pool-a",
		Source:  "proxmox",
		Status:  "online",
		Metadata: map[string]string{
			"proxmox_type": "storage",
			"node":         "pve01",
			"storage_id":   "storage/pve01/pool-a",
			"status":       "DEGRADED",
		},
	})
	if err != nil {
		t.Fatalf("failed to seed pool-a asset: %v", err)
	}
	_, err = sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "proxmox-storage-pool-b",
		Type:    "storage-pool",
		Name:    "storage/pve01/pool-b",
		Source:  "proxmox",
		Status:  "online",
		Metadata: map[string]string{
			"proxmox_type": "storage",
			"node":         "pve01",
			"storage_id":   "storage/pve01/pool-b",
			"status":       "ONLINE",
		},
	})
	if err != nil {
		t.Fatalf("failed to seed pool-b asset: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	samples := []telemetry.MetricSample{
		{
			AssetID:     "proxmox-storage-pool-a",
			Metric:      telemetry.MetricDiskUsedPercent,
			Unit:        "percent",
			Value:       86,
			CollectedAt: now.Add(-4 * 24 * time.Hour),
		},
		{
			AssetID:     "proxmox-storage-pool-a",
			Metric:      telemetry.MetricDiskUsedPercent,
			Unit:        "percent",
			Value:       87,
			CollectedAt: now.Add(-3 * 24 * time.Hour),
		},
		{
			AssetID:     "proxmox-storage-pool-a",
			Metric:      telemetry.MetricDiskUsedPercent,
			Unit:        "percent",
			Value:       88,
			CollectedAt: now.Add(-2 * 24 * time.Hour),
		},
		{
			AssetID:     "proxmox-storage-pool-a",
			Metric:      telemetry.MetricDiskUsedPercent,
			Unit:        "percent",
			Value:       89,
			CollectedAt: now.Add(-1 * 24 * time.Hour),
		},
		{
			AssetID:     "proxmox-storage-pool-a",
			Metric:      telemetry.MetricDiskUsedPercent,
			Unit:        "percent",
			Value:       90,
			CollectedAt: now,
		},
	}
	if err := sut.telemetryStore.AppendSamples(context.Background(), samples); err != nil {
		t.Fatalf("failed to append telemetry samples: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/nodes/pve01/disks/zfs":
			_, _ = w.Write([]byte(`{"data":[]}`))
		case "/api2/json/nodes/pve01/storage/pool-a/status":
			_, _ = w.Write([]byte(`{"data":{"total":1000,"used":900,"avail":100}}`))
		case "/api2/json/nodes/pve01/storage/pool-b/status":
			_, _ = w.Write([]byte(`{"data":{"total":1000,"used":400,"avail":600}}`))
		case "/api2/json/nodes/pve01/storage/pool-a/content":
			_, _ = w.Write([]byte(`{"data":[{"volid":"pool-a:backup/vzdump-qemu-101.vma.zst","content":"backup","size":120,"vmid":101}]}`))
		case "/api2/json/nodes/pve01/storage/pool-b/content":
			_, _ = w.Write([]byte(`{"data":[{"volid":"pool-b:subvol-201-disk-0","content":"rootdir","size":80,"vmid":201}]}`))
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
		7*24*time.Hour,
	)
	if err != nil {
		t.Fatalf("loadProxmoxStorageInsights summary/sort failed: %v", err)
	}
	if len(resp.Pools) != 2 {
		t.Fatalf("expected two pools in summary/sort test, got %d", len(resp.Pools))
	}
	if resp.Pools[0].Name != "pool-a" {
		t.Fatalf("expected higher-risk pool-a to sort first, got %+v", resp.Pools)
	}
	if resp.Summary.DegradedPools != 1 {
		t.Fatalf("expected 1 degraded pool, got %d", resp.Summary.DegradedPools)
	}
	if resp.Summary.HotPools != 1 {
		t.Fatalf("expected 1 hot pool, got %d", resp.Summary.HotPools)
	}
	if resp.Summary.PredictedFullLT30D != 1 {
		t.Fatalf("expected 1 predicted-full-<30d pool, got %d", resp.Summary.PredictedFullLT30D)
	}
	if resp.Summary.StaleTelemetry < 1 {
		t.Fatalf("expected stale-telemetry summary to increment, got %d", resp.Summary.StaleTelemetry)
	}
}

func TestBuildProxmoxStorageInsightEventsUnmappedAndTruncated(t *testing.T) {
	now := time.Unix(1_700_100_000, 0).UTC()
	tasks := make([]proxmox.Task, 0, 100)
	for i := 0; i < 100; i++ {
		tasks = append(tasks, proxmox.Task{
			UPID:       fmt.Sprintf("UPID:pve01:%03d:001:001:vzdump:%d:root@pam:", i, 100+i),
			Node:       "pve01",
			ID:         fmt.Sprintf("%d", 100+i),
			Type:       "zfs-scrub",
			Status:     "stopped",
			ExitStatus: "OK",
			StartTime:  float64(now.Add(-time.Duration(i) * time.Minute).Unix()),
		})
	}

	events := buildProxmoxStorageInsightEvents(tasks, nil, now, 24*time.Hour)
	if len(events) != 80 {
		t.Fatalf("expected event list to be truncated to 80 entries, got %d", len(events))
	}
	if events[0].Pool != "" {
		t.Fatalf("expected unmapped events to keep empty pool, got %+v", events[0])
	}
	if events[0].TaskType != "zfs-scrub" {
		t.Fatalf("expected event task type to be preserved, got %+v", events[0])
	}
}

func TestBuildProxmoxStorageInsightEventsAndIndexEdgeBranches(t *testing.T) {
	now := time.Unix(1_700_200_000, 0).UTC()
	states := []proxmoxStoragePoolState{
		{
			PoolName: "",
			Content: []proxmox.StorageContent{
				{VMID: 101},
			},
		},
		{
			PoolName: "pool-a",
			Content: []proxmox.StorageContent{
				{VMID: 0},
				{VMID: 101},
				{VMID: 101},
			},
		},
		{
			PoolName: "pool-b",
			Content: []proxmox.StorageContent{
				{VMID: 101},
			},
		},
	}

	index := proxmoxpkg.BuildProxmoxStoragePoolIndexByVMID(states)
	if len(index[101]) != 2 || index[101][0] != "pool-a" || index[101][1] != "pool-b" {
		t.Fatalf("expected deduped/sorted pool mapping for VMID 101, got %+v", index)
	}

	tasks := []proxmox.Task{
		{
			UPID:       "UPID:pve01:001:001:001:vzdump:101:root@pam:",
			Node:       "pve01",
			ID:         "101",
			Type:       "vzdump",
			Status:     "running",
			StartTime:  float64(now.Unix()),
			ExitStatus: "",
		},
		{
			UPID:       "UPID:pve01:001:001:001:vzdump:101:root@pam:",
			Node:       "pve01",
			ID:         "101",
			Type:       "vzdump",
			Status:     "stopped",
			ExitStatus: "OK",
			StartTime:  float64(now.Unix()),
		},
		{
			UPID:      "",
			Node:      "pve01",
			ID:        "",
			Type:      "zfs-scrub",
			Status:    "stopped",
			StartTime: float64(now.Unix()),
		},
	}
	events := buildProxmoxStorageInsightEvents(tasks, states, now, 0)
	if len(events) < 4 {
		t.Fatalf("expected mapped events across pools plus unmapped scrub event, got %+v", events)
	}
	if proxmoxpkg.ProxmoxTaskVMID(proxmox.Task{}) != 0 {
		t.Fatalf("expected empty task to map VMID=0")
	}
}

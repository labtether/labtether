package main

import (
	"context"
	"errors"
	"fmt"
	proxmoxpkg "github.com/labtether/labtether/internal/hubapi/proxmox"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/connectors/proxmox"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/telemetry"
)

type proxmoxAssetStoreWithErrors struct {
	inner   persistence.AssetStore
	getErr  error
	listErr error
}

type proxmoxTelemetryStoreWithSeriesError struct {
	inner       persistence.TelemetryStore
	failingID   string
	seriesError error
}

func (s *proxmoxAssetStoreWithErrors) UpsertAssetHeartbeat(req assets.HeartbeatRequest) (assets.Asset, error) {
	return s.inner.UpsertAssetHeartbeat(req)
}

func (s *proxmoxAssetStoreWithErrors) UpdateAsset(id string, req assets.UpdateRequest) (assets.Asset, error) {
	return s.inner.UpdateAsset(id, req)
}

func (s *proxmoxAssetStoreWithErrors) ListAssets() ([]assets.Asset, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.inner.ListAssets()
}

func (s *proxmoxAssetStoreWithErrors) GetAsset(id string) (assets.Asset, bool, error) {
	if s.getErr != nil {
		return assets.Asset{}, false, s.getErr
	}
	return s.inner.GetAsset(id)
}

func (s *proxmoxAssetStoreWithErrors) DeleteAsset(id string) error {
	return s.inner.DeleteAsset(id)
}

func (s *proxmoxTelemetryStoreWithSeriesError) AppendSamples(ctx context.Context, samples []telemetry.MetricSample) error {
	return s.inner.AppendSamples(ctx, samples)
}

func (s *proxmoxTelemetryStoreWithSeriesError) Snapshot(assetID string, at time.Time) (telemetry.Snapshot, error) {
	return s.inner.Snapshot(assetID, at)
}

func (s *proxmoxTelemetryStoreWithSeriesError) Series(assetID string, start, end time.Time, step time.Duration) ([]telemetry.Series, error) {
	if assetID == s.failingID && s.seriesError != nil {
		return nil, s.seriesError
	}
	return s.inner.Series(assetID, start, end, step)
}

func TestSortProxmoxSnapshots(t *testing.T) {
	snapshots := []proxmox.Snapshot{
		{Name: "current", SnapTime: 10},
		{Name: "older", SnapTime: 20},
		{Name: "newer", SnapTime: 30},
	}

	sorted := sortProxmoxSnapshots(snapshots)
	if len(sorted) != 2 {
		t.Fatalf("expected 2 snapshots, got %d", len(sorted))
	}
	if sorted[0].Name != "newer" || sorted[1].Name != "older" {
		t.Fatalf("unexpected sort order: %+v", sorted)
	}
}

func TestFilterAndSortProxmoxTasks(t *testing.T) {
	tasks := []proxmox.Task{
		{
			UPID:      "UPID:pve02:001:001:001:qmstart:101:root@pam:",
			Node:      "pve02",
			ID:        "101",
			StartTime: 200,
		},
		{
			UPID:      "UPID:pve01:001:001:001:qmstart:101:root@pam:",
			Node:      "pve01",
			ID:        "101",
			StartTime: 100,
		},
		{
			UPID:      "UPID:pve01:001:001:001:qmstop:100:root@pam:",
			Node:      "pve01",
			ID:        "100",
			StartTime: 300,
		},
	}

	filtered := filterAndSortProxmoxTasks(tasks, "pve01", "101", 10)
	if len(filtered) != 1 {
		t.Fatalf("expected 1 filtered task, got %d", len(filtered))
	}
	if filtered[0].Node != "pve01" || filtered[0].ID != "101" {
		t.Fatalf("unexpected filtered task: %+v", filtered[0])
	}
}

func TestSelectProxmoxHAForVMAndNode(t *testing.T) {
	resources := []proxmox.HAResource{
		{SID: "vm:101", Node: "pve01", State: "started"},
		{SID: "ct:202", Node: "pve01", State: "started"},
		{SID: "vm:999", Node: "pve02", State: "started"},
	}

	vmTarget := proxmoxSessionTarget{Kind: "qemu", VMID: "101", Node: "pve01"}
	match, related := selectProxmoxHA(resources, vmTarget)
	if match == nil || match.SID != "vm:101" {
		t.Fatalf("expected vm ha match, got %+v", match)
	}
	if len(related) != 1 {
		t.Fatalf("expected 1 related vm ha resource, got %d", len(related))
	}

	nodeTarget := proxmoxSessionTarget{Kind: "node", Node: "pve01"}
	match, related = selectProxmoxHA(resources, nodeTarget)
	if match == nil || match.Node != "pve01" {
		t.Fatalf("expected node ha match for pve01, got %+v", match)
	}
	if len(related) != 2 {
		t.Fatalf("expected 2 related node ha resources, got %d", len(related))
	}
}

func TestLoadProxmoxAssetDetails(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/version":
			_, _ = w.Write([]byte(`{"data":{"release":"8.2"}}`))
		case "/api2/json/nodes/pve01/qemu/101/config":
			_, _ = w.Write([]byte(`{"data":{"name":"web-01","cores":4}}`))
		case "/api2/json/nodes/pve01/qemu/101/snapshot":
			_, _ = w.Write([]byte(`{"data":[{"name":"current"},{"name":"snap-new","snaptime":20},{"name":"snap-old","snaptime":10}]}`))
		case "/api2/json/nodes/pve01/tasks":
			_, _ = w.Write([]byte(`{"data":[{"upid":"UPID:pve01:001:001:001:qmstart:101:root@pam:","node":"pve01","id":"101","type":"qmstart","status":"stopped","exitstatus":"OK","starttime":100}]}`))
		case "/api2/json/cluster/ha/resources":
			_, _ = w.Write([]byte(`{"data":[{"sid":"vm:101","node":"pve01","state":"started","group":"prod"}]}`))
		case "/api2/json/nodes/pve01/qemu/101/firewall/rules":
			_, _ = w.Write([]byte(`{"data":[{"pos":0,"type":"in","action":"ACCEPT","proto":"tcp","dport":"22","enable":1,"comment":"SSH"}]}`))
		case "/api2/json/cluster/backup":
			_, _ = w.Write([]byte(`{"data":[{"id":"backup-0001","schedule":"sat 02:00","storage":"local","mode":"snapshot","compress":"zstd","enabled":1,"vmid":"101","comment":"Weekly backup"}]}`))
		case "/api2/json/cluster/ceph/status":
			// Simulate no Ceph — return 500 (non-fatal, silently skipped).
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"errors":{"node":"no ceph"}}`))
		case "/api2/json/cluster/ceph/osd":
			// Simulate no Ceph — return 500 (non-fatal, silently skipped).
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"errors":{"node":"no ceph"}}`))
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

	srv := &apiServer{}
	target := proxmoxSessionTarget{
		Kind:        "qemu",
		Node:        "pve01",
		VMID:        "101",
		CollectorID: "collector-1",
	}
	runtime := proxmoxpkg.NewProxmoxRuntimeWithCollector(client, "collector-1")

	details, err := srv.loadProxmoxAssetDetails(context.Background(), "proxmox-vm-101", target, runtime)
	if err != nil {
		t.Fatalf("loadProxmoxAssetDetails failed: %v", err)
	}
	if details.Kind != "qemu" || details.Node != "pve01" || details.VMID != "101" {
		t.Fatalf("unexpected detail identity payload: %+v", details)
	}
	if details.Config["name"] != "web-01" {
		t.Fatalf("unexpected config payload: %+v", details.Config)
	}
	if len(details.Snapshots) != 2 || details.Snapshots[0].Name != "snap-new" {
		t.Fatalf("unexpected snapshots payload: %+v", details.Snapshots)
	}
	if len(details.Tasks) != 1 || details.Tasks[0].ID != "101" {
		t.Fatalf("unexpected tasks payload: %+v", details.Tasks)
	}
	if details.HA.Match == nil || details.HA.Match.SID != "vm:101" {
		t.Fatalf("unexpected ha payload: %+v", details.HA)
	}
	if len(details.FirewallRules) != 1 || details.FirewallRules[0].Action != "ACCEPT" || details.FirewallRules[0].Dport != "22" {
		t.Fatalf("unexpected firewall rules payload: %+v", details.FirewallRules)
	}
	if len(details.BackupSchedules) != 1 || details.BackupSchedules[0].ID != "backup-0001" || details.BackupSchedules[0].Schedule != "sat 02:00" {
		t.Fatalf("unexpected backup schedules payload: %+v", details.BackupSchedules)
	}
	if len(details.Warnings) != 0 {
		t.Fatalf("did not expect warnings, got: %+v", details.Warnings)
	}
}

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

func TestParseStorageInsightsWindow(t *testing.T) {
	if got := parseStorageInsightsWindow("7d"); got != 7*24*time.Hour {
		t.Fatalf("parseStorageInsightsWindow(7d) = %s", got)
	}
	if got := parseStorageInsightsWindow("36h"); got != 36*time.Hour {
		t.Fatalf("parseStorageInsightsWindow(36h) = %s", got)
	}
	if got := parseStorageInsightsWindow("1h"); got != 7*24*time.Hour {
		t.Fatalf("parseStorageInsightsWindow(1h) should clamp to fallback 7d, got %s", got)
	}
}

func TestProxmoxStorageAssetBelongsToNode(t *testing.T) {
	assetWithNodeMetadata := assets.Asset{
		Metadata: map[string]string{
			"node":       "pve01",
			"storage_id": "storage/pve01/local-zfs",
		},
	}
	if !proxmoxpkg.ProxmoxStorageAssetBelongsToNode(assetWithNodeMetadata, "pve01") {
		t.Fatalf("expected storage asset with node metadata to belong to pve01")
	}

	assetByStorageID := assets.Asset{
		Metadata: map[string]string{
			"storage_id": "storage/pve02/fast-ssd",
		},
	}
	if !proxmoxpkg.ProxmoxStorageAssetBelongsToNode(assetByStorageID, "pve02") {
		t.Fatalf("expected storage/pve02/fast-ssd to belong to pve02")
	}
	if proxmoxpkg.ProxmoxStorageAssetBelongsToNode(assetByStorageID, "pve01") {
		t.Fatalf("did not expect storage/pve02/fast-ssd to belong to pve01")
	}

	legacyStorageID := assets.Asset{
		Metadata: map[string]string{
			"storage_id": "pve03/archive",
		},
	}
	if !proxmoxpkg.ProxmoxStorageAssetBelongsToNode(legacyStorageID, "pve03") {
		t.Fatalf("expected legacy storage id pve03/archive to belong to pve03")
	}

	if proxmoxpkg.ProxmoxStorageAssetBelongsToNode(assets.Asset{Metadata: map[string]string{"node": "pve01"}}, "") {
		t.Fatalf("did not expect empty target node to match")
	}
	if proxmoxpkg.ProxmoxStorageAssetBelongsToNode(assets.Asset{Metadata: map[string]string{}}, "pve01") {
		t.Fatalf("did not expect asset with no node/storage metadata to match")
	}
	prefixOnly := assets.Asset{Metadata: map[string]string{"storage_id": "pve04/archive"}}
	if !proxmoxpkg.ProxmoxStorageAssetBelongsToNode(prefixOnly, "pve04") {
		t.Fatalf("expected two-segment storage id to match pve04")
	}
	if proxmoxpkg.ProxmoxStorageAssetBelongsToNode(assets.Asset{Metadata: map[string]string{"storage_id": "local-zfs"}}, "pve01") {
		t.Fatalf("did not expect single-segment storage id to match unrelated node")
	}
}

func TestParseMetadataFloatAndParseAnyInt64(t *testing.T) {
	metadata := map[string]string{
		"used_percent": "84.5",
		"invalid":      "nope",
	}
	used, ok := proxmoxpkg.ParseMetadataFloat(metadata, "invalid", "used_percent")
	if !ok || used != 84.5 {
		t.Fatalf("expected proxmoxpkg.ParseMetadataFloat to resolve 84.5, got value=%v ok=%v", used, ok)
	}
	if _, ok := proxmoxpkg.ParseMetadataFloat(metadata, "missing"); ok {
		t.Fatalf("expected proxmoxpkg.ParseMetadataFloat to fail for missing key")
	}

	cases := []struct {
		value any
		want  int64
		ok    bool
	}{
		{value: int64(42), want: 42, ok: true},
		{value: int32(43), want: 43, ok: true},
		{value: int(44), want: 44, ok: true},
		{value: float64(45.9), want: 45, ok: true},
		{value: float32(46.9), want: 46, ok: true},
		{value: "47.1", want: 47, ok: true},
		{value: "nope", want: 0, ok: false},
	}
	for _, tc := range cases {
		got, ok := parseAnyInt64(tc.value)
		if ok != tc.ok || got != tc.want {
			t.Fatalf("parseAnyInt64(%v) => (%d,%v), expected (%d,%v)", tc.value, got, ok, tc.want, tc.ok)
		}
	}
}

func TestProxmoxTaskVMIDAndRouteDispatch(t *testing.T) {
	if got := proxmoxpkg.ProxmoxTaskVMID(proxmox.Task{ID: "101"}); got != 101 {
		t.Fatalf("expected vmid from id field 101, got %d", got)
	}
	if got := proxmoxpkg.ProxmoxTaskVMID(proxmox.Task{UPID: "UPID:pve01:001:001:001:vzdump:202:root@pam:"}); got != 202 {
		t.Fatalf("expected vmid from upid field 202, got %d", got)
	}
	if got := proxmoxpkg.ProxmoxTaskVMID(proxmox.Task{ID: "bad", UPID: "UPID:bad"}); got != 0 {
		t.Fatalf("expected vmid parse fallback to 0, got %d", got)
	}

	sut := newTestAPIServer(t)

	req := httptest.NewRequest(http.MethodGet, "/proxmox/tasks/pve01/UPID-1/unknown", nil)
	rec := httptest.NewRecorder()
	sut.handleProxmoxTaskRoutes(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown task route, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/proxmox/nodes/pve01/unknown", nil)
	rec = httptest.NewRecorder()
	sut.handleProxmoxNodeRoutes(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown node route, got %d", rec.Code)
	}
}

func TestHandleProxmoxAssetsGuards(t *testing.T) {
	sut := newTestAPIServer(t)

	req := httptest.NewRequest(http.MethodGet, "/proxmox/assets/", nil)
	rec := httptest.NewRecorder()
	sut.handleProxmoxAssets(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing asset path, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/proxmox/assets/asset-1/details", nil)
	rec = httptest.NewRecorder()
	sut.handleProxmoxAssets(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for non-GET, got %d", rec.Code)
	}

	_, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "agent-host-01",
		Type:    "server",
		Name:    "agent-host-01",
		Source:  "agent",
		Status:  "online",
	})
	if err != nil {
		t.Fatalf("failed to seed non-proxmox asset: %v", err)
	}

	req = httptest.NewRequest(http.MethodGet, "/proxmox/assets/agent-host-01/details", nil)
	rec = httptest.NewRecorder()
	sut.handleProxmoxAssets(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for non-proxmox asset, got %d", rec.Code)
	}
}

func TestHandleProxmoxAssetsDetailsSuccess(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/version":
			_, _ = w.Write([]byte(`{"data":{"release":"8.3"}}`))
		case "/api2/json/nodes/pve01/status":
			_, _ = w.Write([]byte(`{"data":{"status":"online","cpu":0.2}}`))
		case "/api2/json/nodes/pve01/tasks":
			_, _ = w.Write([]byte(`{"data":[]}`))
		case "/api2/json/cluster/ha/resources":
			_, _ = w.Write([]byte(`{"data":[]}`))
		case "/api2/json/nodes/pve01/firewall/rules":
			_, _ = w.Write([]byte(`{"data":[]}`))
		case "/api2/json/cluster/backup":
			_, _ = w.Write([]byte(`{"data":[]}`))
		case "/api2/json/cluster/ceph/status", "/api2/json/cluster/ceph/osd":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"errors":{"node":"no ceph"}}`))
		case "/api2/json/nodes/pve01/disks/zfs":
			_, _ = w.Write([]byte(`{"data":[]}`))
		default:
			t.Fatalf("unexpected proxmox path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	sut := newTestAPIServer(t)
	configureSingleProxmoxCollector(t, sut, server.URL, "collector-proxmox-1")

	_, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "proxmox-node-pve01",
		Type:    "hypervisor-node",
		Name:    "pve01",
		Source:  "proxmox",
		Status:  "online",
		Metadata: map[string]string{
			"proxmox_type": "node",
			"node":         "pve01",
			"collector_id": "collector-proxmox-1",
		},
	})
	if err != nil {
		t.Fatalf("failed to seed proxmox node asset: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/proxmox/assets/proxmox-node-pve01/details", nil)
	rec := httptest.NewRecorder()
	sut.handleProxmoxAssets(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, strings.TrimSpace(rec.Body.String()))
	}
	if !strings.Contains(rec.Body.String(), `"asset_id":"proxmox-node-pve01"`) {
		t.Fatalf("expected proxmox details payload, got %s", rec.Body.String())
	}
}

func TestHandleProxmoxAssetsAdditionalErrorBranches(t *testing.T) {
	t.Run("path parsing and resolve errors", func(t *testing.T) {
		sut := newTestAPIServer(t)

		req := httptest.NewRequest(http.MethodGet, "/proxmox/assets//details", nil)
		rec := httptest.NewRecorder()
		sut.handleProxmoxAssets(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for empty asset id, got %d", rec.Code)
		}

		req = httptest.NewRequest(http.MethodGet, "/proxmox/assets/proxmox-node-pve01", nil)
		rec = httptest.NewRecorder()
		sut.handleProxmoxAssets(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for missing asset action, got %d", rec.Code)
		}

		sut.assetStore = &proxmoxAssetStoreWithErrors{
			inner:  sut.assetStore,
			getErr: errors.New("asset store unavailable"),
		}
		req = httptest.NewRequest(http.MethodGet, "/proxmox/assets/proxmox-node-pve01/details", nil)
		rec = httptest.NewRecorder()
		sut.handleProxmoxAssets(rec, req)
		if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "An internal error occurred.") {
			t.Fatalf("expected sanitized error 502, got %d body=%s", rec.Code, strings.TrimSpace(rec.Body.String()))
		}
	})

	t.Run("details and storage insights load failures", func(t *testing.T) {
		detailsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api2/json/version":
				_, _ = w.Write([]byte(`{"data":{"release":"8.3"}}`))
			case "/api2/json/nodes/pve01/status":
				w.WriteHeader(http.StatusBadGateway)
				_, _ = w.Write([]byte(`{"errors":"status failed"}`))
			case "/api2/json/nodes/pve01/tasks", "/api2/json/cluster/ha/resources", "/api2/json/nodes/pve01/firewall/rules", "/api2/json/cluster/backup":
				_, _ = w.Write([]byte(`{"data":[]}`))
			case "/api2/json/cluster/ceph/status", "/api2/json/cluster/ceph/osd":
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"errors":{"node":"no ceph"}}`))
			case "/api2/json/nodes/pve01/disks/zfs":
				_, _ = w.Write([]byte(`{"data":[]}`))
			default:
				t.Fatalf("unexpected proxmox path: %s", r.URL.Path)
			}
		}))
		defer detailsServer.Close()

		sut := newTestAPIServer(t)
		configureSingleProxmoxCollector(t, sut, detailsServer.URL, "collector-proxmox-1")

		_, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
			AssetID: "proxmox-node-pve01",
			Type:    "hypervisor-node",
			Name:    "pve01",
			Source:  "proxmox",
			Status:  "online",
			Metadata: map[string]string{
				"proxmox_type": "node",
				"node":         "pve01",
				"collector_id": "collector-proxmox-1",
			},
		})
		if err != nil {
			t.Fatalf("failed to seed proxmox node asset: %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/proxmox/assets/proxmox-node-pve01/details", nil)
		rec := httptest.NewRecorder()
		sut.handleProxmoxAssets(rec, req)
		if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "An internal error occurred.") {
			t.Fatalf("expected sanitized error 502, got %d body=%s", rec.Code, strings.TrimSpace(rec.Body.String()))
		}

		sut.assetStore = &proxmoxAssetStoreWithErrors{
			inner:   sut.assetStore,
			listErr: errors.New("list assets failed"),
		}
		req = httptest.NewRequest(http.MethodGet, "/proxmox/assets/proxmox-node-pve01/storage/insights", nil)
		rec = httptest.NewRecorder()
		sut.handleProxmoxAssets(rec, req)
		if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "An internal error occurred.") {
			t.Fatalf("expected sanitized error 502, got %d body=%s", rec.Code, strings.TrimSpace(rec.Body.String()))
		}
		// Restore the underlying store so global cleanup hooks keep working.
		sut.assetStore = sut.assetStore.(*proxmoxAssetStoreWithErrors).inner
	})
}

func TestHandleProxmoxTaskLogUsesCollectorQueryParam(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)

	var collectorOneCalls atomic.Int32

	collectorOne := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		collectorOneCalls.Add(1)
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"errors":"wrong collector selected"}`))
	}))
	defer collectorOne.Close()

	collectorTwo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/nodes/pve02/tasks/UPID-2/log":
			if got := r.URL.Query().Get("limit"); got != "500" {
				t.Fatalf("expected limit=500 query, got %q", got)
			}
			_, _ = w.Write([]byte(`{"data":[{"n":1,"t":"line-one"},{"n":2,"t":"line-two"}]}`))
		default:
			t.Fatalf("unexpected proxmox request path: %s", r.URL.Path)
		}
	}))
	defer collectorTwo.Close()

	sut := newTestAPIServer(t)
	configureDualProxmoxCollectors(t, sut, collectorOne.URL, collectorTwo.URL)

	req := httptest.NewRequest(http.MethodGet, "/proxmox/tasks/pve02/UPID-2/log?collector_id=collector-proxmox-2", nil)
	rec := httptest.NewRecorder()
	sut.handleProxmoxTaskLog(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, strings.TrimSpace(rec.Body.String()))
	}
	if !strings.Contains(rec.Body.String(), "line-one\\nline-two\\n") {
		t.Fatalf("expected proxmox task log payload, got %s", rec.Body.String())
	}
	if collectorOneCalls.Load() != 0 {
		t.Fatalf("expected collector one to receive no requests, got %d", collectorOneCalls.Load())
	}
}

func TestHandleProxmoxTaskStopUsesCollectorQueryParam(t *testing.T) {
	var collectorOneCalls atomic.Int32

	collectorOne := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		collectorOneCalls.Add(1)
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"errors":"wrong collector selected"}`))
	}))
	defer collectorOne.Close()

	collectorTwo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Fatalf("expected DELETE, got %s", r.Method)
		}
		switch r.URL.Path {
		case "/api2/json/nodes/pve02/tasks/UPID-2":
			_, _ = w.Write([]byte(`{"data":"OK"}`))
		default:
			t.Fatalf("unexpected proxmox request path: %s", r.URL.Path)
		}
	}))
	defer collectorTwo.Close()

	sut := newTestAPIServer(t)
	configureDualProxmoxCollectors(t, sut, collectorOne.URL, collectorTwo.URL)

	req := httptest.NewRequest(http.MethodPost, "/proxmox/tasks/pve02/UPID-2/stop?collector_id=collector-proxmox-2", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "owner", "owner"))
	rec := httptest.NewRecorder()
	sut.handleProxmoxTaskStop(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, strings.TrimSpace(rec.Body.String()))
	}
	if !strings.Contains(rec.Body.String(), `"status":"stopped"`) {
		t.Fatalf("expected stopped status payload, got %s", rec.Body.String())
	}
	if collectorOneCalls.Load() != 0 {
		t.Fatalf("expected collector one to receive no requests, got %d", collectorOneCalls.Load())
	}
}

func TestHandleProxmoxClusterStatusUsesCollectorQueryParam(t *testing.T) {
	var collectorOneCalls atomic.Int32

	collectorOne := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		collectorOneCalls.Add(1)
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"errors":"wrong collector selected"}`))
	}))
	defer collectorOne.Close()

	collectorTwo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/cluster/status":
			_, _ = w.Write([]byte(`{"data":[{"type":"node","name":"pve02","online":1}]}`))
		default:
			t.Fatalf("unexpected proxmox request path: %s", r.URL.Path)
		}
	}))
	defer collectorTwo.Close()

	sut := newTestAPIServer(t)
	configureDualProxmoxCollectors(t, sut, collectorOne.URL, collectorTwo.URL)

	req := httptest.NewRequest(http.MethodGet, "/proxmox/cluster/status?collector_id=collector-proxmox-2", nil)
	rec := httptest.NewRecorder()
	sut.handleProxmoxClusterStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, strings.TrimSpace(rec.Body.String()))
	}
	if !strings.Contains(rec.Body.String(), `"pve02"`) {
		t.Fatalf("expected cluster status payload from collector two, got %s", rec.Body.String())
	}
	if collectorOneCalls.Load() != 0 {
		t.Fatalf("expected collector one to receive no requests, got %d", collectorOneCalls.Load())
	}
}

func TestHandleProxmoxNodeNetworkUsesCollectorQueryParam(t *testing.T) {
	var collectorOneCalls atomic.Int32

	collectorOne := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		collectorOneCalls.Add(1)
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"errors":"wrong collector selected"}`))
	}))
	defer collectorOne.Close()

	collectorTwo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/nodes/pve02/network":
			_, _ = w.Write([]byte(`{"data":[{"iface":"vmbr0","active":1}]}`))
		default:
			t.Fatalf("unexpected proxmox request path: %s", r.URL.Path)
		}
	}))
	defer collectorTwo.Close()

	sut := newTestAPIServer(t)
	configureDualProxmoxCollectors(t, sut, collectorOne.URL, collectorTwo.URL)

	req := httptest.NewRequest(http.MethodGet, "/proxmox/nodes/pve02/network?collector_id=collector-proxmox-2", nil)
	rec := httptest.NewRecorder()
	sut.handleProxmoxNodeNetwork(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, strings.TrimSpace(rec.Body.String()))
	}
	if !strings.Contains(rec.Body.String(), `"vmbr0"`) {
		t.Fatalf("expected proxmox node network payload from collector two, got %s", rec.Body.String())
	}
	if collectorOneCalls.Load() != 0 {
		t.Fatalf("expected collector one to receive no requests, got %d", collectorOneCalls.Load())
	}
}

func TestProxmoxStoragePoolNameFromAsset(t *testing.T) {
	if got := proxmoxpkg.ProxmoxStoragePoolNameFromAsset(assets.Asset{
		Metadata: map[string]string{"storage_id": "storage/pve01/local-zfs"},
		Name:     "storage/pve01/wrong",
	}); got != "local-zfs" {
		t.Fatalf("expected pool name from storage_id, got %q", got)
	}

	if got := proxmoxpkg.ProxmoxStoragePoolNameFromAsset(assets.Asset{
		Metadata: map[string]string{"storage_id": "storage/pve01/"},
		Name:     "storage/pve01/fallback",
	}); got != "fallback" {
		t.Fatalf("expected fallback pool name from asset name, got %q", got)
	}

	if got := proxmoxpkg.ProxmoxStoragePoolNameFromAsset(assets.Asset{
		Name: "plain-pool",
	}); got != "plain-pool" {
		t.Fatalf("expected plain name fallback, got %q", got)
	}

	if got := proxmoxpkg.ProxmoxStoragePoolNameFromAsset(assets.Asset{}); got != "" {
		t.Fatalf("expected empty pool name for empty asset, got %q", got)
	}
}

func TestStorageInsightsWindowHelpers(t *testing.T) {
	if got := formatStorageInsightsWindow(48 * time.Hour); got != "2d" {
		t.Fatalf("expected 48h => 2d, got %q", got)
	}
	if got := formatStorageInsightsWindow(90 * time.Minute); got != "1h30m0s" {
		t.Fatalf("expected 90m => 1h30m0s, got %q", got)
	}

	if got := proxmoxpkg.StorageInsightsStep(8 * 24 * time.Hour); got != time.Hour {
		t.Fatalf("expected >=7d window to use 1h step, got %s", got)
	}
	if got := proxmoxpkg.StorageInsightsStep(24 * time.Hour); got != 30*time.Minute {
		t.Fatalf("expected >=24h window to use 30m step, got %s", got)
	}
	if got := proxmoxpkg.StorageInsightsStep(6 * time.Hour); got != 5*time.Minute {
		t.Fatalf("expected short window to use 5m step, got %s", got)
	}
}

func TestProxmoxTaskTimelineStorageRelevanceAndMessages(t *testing.T) {
	if got := proxmoxpkg.ProxmoxTaskTimelineTS(proxmox.Task{EndTime: 200, StartTime: 100}); got != 200 {
		t.Fatalf("expected EndTime priority, got %d", got)
	}
	if got := proxmoxpkg.ProxmoxTaskTimelineTS(proxmox.Task{StartTime: 150}); got != 150 {
		t.Fatalf("expected StartTime fallback, got %d", got)
	}
	if got := proxmoxpkg.ProxmoxTaskTimelineTS(proxmox.Task{}); got != 0 {
		t.Fatalf("expected empty timeline ts 0, got %d", got)
	}

	if !proxmoxpkg.ProxmoxTaskIsStorageRelevant(proxmox.Task{Type: "zfs-scrub"}, false) {
		t.Fatalf("expected zfs task type to always be storage relevant")
	}
	if !proxmoxpkg.ProxmoxTaskIsStorageRelevant(proxmox.Task{Type: ""}, true) {
		t.Fatalf("expected empty task type to be storage relevant when workload mapped")
	}
	if proxmoxpkg.ProxmoxTaskIsStorageRelevant(proxmox.Task{Type: "vzdump"}, false) {
		t.Fatalf("did not expect workload task without mapped workload to be storage relevant")
	}
	if !proxmoxpkg.ProxmoxTaskIsStorageRelevant(proxmox.Task{Type: "vzdump"}, true) {
		t.Fatalf("expected workload task with mapped workload to be storage relevant")
	}
	if proxmoxpkg.ProxmoxTaskIsStorageRelevant(proxmox.Task{Type: "qmstart"}, true) {
		t.Fatalf("did not expect unrelated task type to be storage relevant")
	}

	if got := proxmoxpkg.ProxmoxStorageTaskSeverity(proxmox.Task{Status: "running"}); got != "info" {
		t.Fatalf("expected running severity info, got %q", got)
	}
	if got := proxmoxpkg.ProxmoxStorageTaskSeverity(proxmox.Task{Status: "error"}); got != "critical" {
		t.Fatalf("expected error severity critical, got %q", got)
	}
	if got := proxmoxpkg.ProxmoxStorageTaskSeverity(proxmox.Task{Status: "stopped", ExitStatus: "FAIL"}); got != "critical" {
		t.Fatalf("expected failing exitstatus severity critical, got %q", got)
	}

	if got := proxmoxpkg.ProxmoxStorageTaskMessage(proxmox.Task{Type: "vzdump", Status: "running"}, 101); !strings.Contains(got, "running for VM/CT 101") {
		t.Fatalf("unexpected running message: %q", got)
	}
	if got := proxmoxpkg.ProxmoxStorageTaskMessage(proxmox.Task{Type: "vzdump", ExitStatus: "OK"}, 101); !strings.Contains(got, "completed for VM/CT 101") {
		t.Fatalf("unexpected OK completion message: %q", got)
	}
	if got := proxmoxpkg.ProxmoxStorageTaskMessage(proxmox.Task{Type: "vzdump", ExitStatus: "failed"}, 0); got != "vzdump finished with failed" {
		t.Fatalf("unexpected failure message: %q", got)
	}
	if got := proxmoxpkg.ProxmoxStorageTaskMessage(proxmox.Task{Type: "vzdump", Status: "stopped"}, 0); got != "vzdump status stopped" {
		t.Fatalf("unexpected explicit status message: %q", got)
	}
	if got := proxmoxpkg.ProxmoxStorageTaskMessage(proxmox.Task{}, 0); got != "task completed" {
		t.Fatalf("unexpected default task message: %q", got)
	}
}

func TestAnalyzeDiskGrowthAndStorageRiskHelpers(t *testing.T) {
	emptyRate, emptyConfidence, emptyLatest := proxmoxpkg.AnalyzeDiskGrowth(nil)
	if emptyRate != 0 || emptyConfidence != "low" || emptyLatest != 0 {
		t.Fatalf("unexpected empty growth analysis result: rate=%v confidence=%s latest=%d", emptyRate, emptyConfidence, emptyLatest)
	}

	singleRate, singleConfidence, singleLatest := proxmoxpkg.AnalyzeDiskGrowth([]telemetry.Point{
		{TS: 1000, Value: 42},
	})
	if singleRate != 0 || singleConfidence != "low" || singleLatest != 1000 {
		t.Fatalf("unexpected single-point growth analysis: rate=%v confidence=%s latest=%d", singleRate, singleConfidence, singleLatest)
	}

	highRate, highConfidence, _ := proxmoxpkg.AnalyzeDiskGrowth([]telemetry.Point{
		{TS: 1 * 24 * 60 * 60, Value: 10},
		{TS: 2 * 24 * 60 * 60, Value: 11},
		{TS: 3 * 24 * 60 * 60, Value: 12},
		{TS: 4 * 24 * 60 * 60, Value: 13},
		{TS: 5 * 24 * 60 * 60, Value: 14},
		{TS: 6 * 24 * 60 * 60, Value: 15},
		{TS: 7 * 24 * 60 * 60, Value: 16},
	})
	if highRate <= 0 || highConfidence != "high" {
		t.Fatalf("expected stable positive growth with high confidence, got rate=%v confidence=%s", highRate, highConfidence)
	}

	used := 95.0
	daysToFull := 5.0
	score, state, reasons := proxmoxpkg.ComputeStorageRisk(proxmoxpkg.ProxmoxStorageInsightPool{
		Health:      "DEGRADED",
		UsedPercent: &used,
		Forecast: proxmoxpkg.ProxmoxStorageForecast{
			DaysToFull: &daysToFull,
		},
		TelemetryStale: true,
	})
	if score != 100 || state != "critical" {
		t.Fatalf("expected capped critical score, got score=%d state=%s reasons=%v", score, state, reasons)
	}
	if len(reasons) == 0 {
		t.Fatalf("expected non-empty risk reasons")
	}

	healthyScore, healthyState, healthyReasons := proxmoxpkg.ComputeStorageRisk(proxmoxpkg.ProxmoxStorageInsightPool{
		Health: "ONLINE",
	})
	if healthyScore != 0 || healthyState != "healthy" {
		t.Fatalf("expected healthy risk baseline, got score=%d state=%s reasons=%v", healthyScore, healthyState, healthyReasons)
	}
	if len(healthyReasons) != 1 || !strings.Contains(strings.ToLower(healthyReasons[0]), "no immediate") {
		t.Fatalf("unexpected healthy reasons payload: %v", healthyReasons)
	}

	if !proxmoxpkg.ProxmoxStorageHealthOK("ok") || proxmoxpkg.ProxmoxStorageHealthOK("degraded") {
		t.Fatalf("unexpected proxmoxpkg.ProxmoxStorageHealthOK behavior")
	}
}

func TestBuildProxmoxStoragePoolStatesBranches(t *testing.T) {
	assetList := []assets.Asset{
		{
			ID:     "pool-1",
			Type:   "storage-pool",
			Name:   "storage/pve01/local-zfs",
			Source: "proxmox",
			Metadata: map[string]string{
				"storage_id": "storage/pve01/local-zfs",
			},
		},
		{
			ID:     "pool-2",
			Type:   "storage-pool",
			Name:   "storage/pve02/fast-ssd",
			Source: "proxmox",
			Metadata: map[string]string{
				"storage_id": "storage/pve02/fast-ssd",
			},
		},
		{
			ID:     "other-1",
			Type:   "vm",
			Name:   "pve01/101",
			Source: "proxmox",
		},
		{
			ID:     "non-proxmox-pool",
			Type:   "storage-pool",
			Name:   "local",
			Source: "agent",
		},
	}

	nodeStates := proxmoxpkg.BuildProxmoxStoragePoolStates(assetList, proxmoxSessionTarget{Kind: "node", Node: "pve01"}, "")
	if len(nodeStates) != 1 || nodeStates[0].PoolName != "local-zfs" || !nodeStates[0].HasAsset {
		t.Fatalf("unexpected node-filtered states: %+v", nodeStates)
	}

	storageStates := proxmoxpkg.BuildProxmoxStoragePoolStates(assetList, proxmoxSessionTarget{
		Kind:        "storage",
		StorageName: "scratch-pool",
	}, "pool-1")
	if len(storageStates) != 2 {
		t.Fatalf("expected requested pool + storage target fallback, got %d states: %+v", len(storageStates), storageStates)
	}

	foundRequested := false
	foundFallback := false
	for _, state := range storageStates {
		switch state.PoolName {
		case "local-zfs":
			foundRequested = state.HasAsset && state.Asset.ID == "pool-1"
		case "scratch-pool":
			foundFallback = !state.HasAsset
		}
	}
	if !foundRequested || !foundFallback {
		t.Fatalf("unexpected storage states (requested=%v fallback=%v): %+v", foundRequested, foundFallback, storageStates)
	}
}

func TestHandleProxmoxRouteAndGuardBranches(t *testing.T) {
	sut := newTestAPIServer(t)

	taskReq := httptest.NewRequest(http.MethodGet, "/proxmox/tasks/", nil)
	taskRec := httptest.NewRecorder()
	sut.handleProxmoxTaskRoutes(taskRec, taskReq)
	if taskRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing task path, got %d", taskRec.Code)
	}

	taskReq = httptest.NewRequest(http.MethodPost, "/proxmox/tasks/pve01/UPID-1/log", nil)
	taskRec = httptest.NewRecorder()
	sut.handleProxmoxTaskRoutes(taskRec, taskReq)
	if taskRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected log route dispatch to enforce method guard (405), got %d", taskRec.Code)
	}

	taskReq = httptest.NewRequest(http.MethodGet, "/proxmox/tasks/pve01/UPID-1/stop", nil)
	taskRec = httptest.NewRecorder()
	sut.handleProxmoxTaskRoutes(taskRec, taskReq)
	if taskRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected stop route dispatch to enforce method guard (405), got %d", taskRec.Code)
	}

	nodeReq := httptest.NewRequest(http.MethodGet, "/proxmox/nodes/", nil)
	nodeRec := httptest.NewRecorder()
	sut.handleProxmoxNodeRoutes(nodeRec, nodeReq)
	if nodeRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing node path, got %d", nodeRec.Code)
	}

	// POST to network is now allowed (create interface) — without a configured
	// collector, it returns 502 (bad gateway) instead of the old 405.
	nodeReq = httptest.NewRequest(http.MethodPost, "/proxmox/nodes/pve01/network", nil)
	nodeRec = httptest.NewRecorder()
	sut.handleProxmoxNodeRoutes(nodeRec, nodeReq)
	if nodeRec.Code != http.StatusBadGateway {
		t.Fatalf("expected network POST to return 502 (no collector), got %d", nodeRec.Code)
	}
}

func TestHandleProxmoxTaskAndNodeHandlersGuardBranches(t *testing.T) {
	sut := newTestAPIServer(t)

	req := httptest.NewRequest(http.MethodPost, "/proxmox/tasks/pve01/UPID-1/log", nil)
	rec := httptest.NewRecorder()
	sut.handleProxmoxTaskLog(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for task log method guard, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/proxmox/tasks/pve01/UPID-1", nil)
	rec = httptest.NewRecorder()
	sut.handleProxmoxTaskLog(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for malformed task log path, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/proxmox/tasks//UPID-1/log", nil)
	rec = httptest.NewRecorder()
	sut.handleProxmoxTaskLog(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing node in task log path, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/proxmox/tasks/pve01/UPID-1/log", nil)
	rec = httptest.NewRecorder()
	sut.handleProxmoxTaskLog(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 when task log runtime is unavailable, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/proxmox/tasks/pve01/UPID-1/stop", nil)
	rec = httptest.NewRecorder()
	sut.handleProxmoxTaskStop(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for task stop method guard, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/proxmox/tasks/pve01/UPID-1", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "owner", "owner"))
	rec = httptest.NewRecorder()
	sut.handleProxmoxTaskStop(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for malformed task stop path, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/proxmox/tasks//UPID-1/stop", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "owner", "owner"))
	rec = httptest.NewRecorder()
	sut.handleProxmoxTaskStop(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing node in task stop path, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/proxmox/tasks/pve01/UPID-1/stop", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "owner", "owner"))
	rec = httptest.NewRecorder()
	sut.handleProxmoxTaskStop(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 when task stop runtime is unavailable, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/proxmox/cluster/status", nil)
	rec = httptest.NewRecorder()
	sut.handleProxmoxClusterStatus(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for cluster status method guard, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/proxmox/cluster/status", nil)
	rec = httptest.NewRecorder()
	sut.handleProxmoxClusterStatus(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 when cluster runtime is unavailable, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/proxmox/nodes/pve01/network", nil)
	rec = httptest.NewRecorder()
	sut.handleProxmoxNodeNetwork(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for node network method guard, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/proxmox/nodes/pve01/iface", nil)
	rec = httptest.NewRecorder()
	sut.handleProxmoxNodeNetwork(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for malformed node network path, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/proxmox/nodes//network", nil)
	rec = httptest.NewRecorder()
	sut.handleProxmoxNodeNetwork(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing node on network path, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/proxmox/nodes/pve01/network", nil)
	rec = httptest.NewRecorder()
	sut.handleProxmoxNodeNetwork(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 when node network runtime is unavailable, got %d", rec.Code)
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

func TestLoadProxmoxAssetDetailsFatalAndWarningBranches(t *testing.T) {
	configErrorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/version":
			_, _ = w.Write([]byte(`{"data":{"release":"8.3"}}`))
		case "/api2/json/nodes/pve01/qemu/101/config":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"errors":{"config":"failed"}}`))
		case "/api2/json/nodes/pve01/qemu/101/snapshot",
			"/api2/json/nodes/pve01/tasks",
			"/api2/json/cluster/ha/resources",
			"/api2/json/nodes/pve01/qemu/101/firewall/rules",
			"/api2/json/cluster/backup",
			"/api2/json/cluster/ceph/status",
			"/api2/json/cluster/ceph/osd":
			_, _ = w.Write([]byte(`{"data":[]}`))
		default:
			t.Fatalf("unexpected proxmox path: %s", r.URL.Path)
		}
	}))
	defer configErrorServer.Close()

	client, err := proxmox.NewClient(proxmox.Config{
		BaseURL:     configErrorServer.URL,
		TokenID:     "id",
		TokenSecret: "secret",
		Timeout:     5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	sut := newTestAPIServer(t)

	_, err = sut.loadProxmoxAssetDetails(
		context.Background(),
		"proxmox-vm-101",
		proxmoxSessionTarget{Kind: "qemu", Node: "pve01", VMID: "101"},
		proxmoxpkg.NewProxmoxRuntimeWithCollector(client, "collector-1"),
	)
	if err == nil {
		t.Fatalf("expected config failure to bubble up as fatal error")
	}

	warningServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/version":
			_, _ = w.Write([]byte(`{"data":{"release":"8.3"}}`))
		case "/api2/json/nodes/pve01/tasks",
			"/api2/json/cluster/ha/resources",
			"/api2/json/cluster/backup",
			"/api2/json/cluster/ceph/status",
			"/api2/json/cluster/ceph/osd":
			_, _ = w.Write([]byte(`{"data":[]}`))
		default:
			t.Fatalf("unexpected proxmox path: %s", r.URL.Path)
		}
	}))
	defer warningServer.Close()

	client, err = proxmox.NewClient(proxmox.Config{
		BaseURL:     warningServer.URL,
		TokenID:     "id",
		TokenSecret: "secret",
		Timeout:     5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	details, err := sut.loadProxmoxAssetDetails(
		context.Background(),
		"proxmox-storage-local-zfs",
		proxmoxSessionTarget{Kind: "storage", Node: "pve01"},
		proxmoxpkg.NewProxmoxRuntimeWithCollector(client, "collector-1"),
	)
	if err != nil {
		t.Fatalf("did not expect error for storage details with missing storage name warning: %v", err)
	}
	if len(details.Warnings) == 0 || !strings.Contains(strings.Join(details.Warnings, " "), "storage name unavailable") {
		t.Fatalf("expected storage-name warning, got %+v", details.Warnings)
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

func TestLoadProxmoxAssetDetailsStorageContentSorting(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/version":
			_, _ = w.Write([]byte(`{"data":{"release":"8.3"}}`))
		case "/api2/json/nodes/pve01/storage/local-zfs/status":
			_, _ = w.Write([]byte(`{"data":{"total":1000,"used":300,"avail":700}}`))
		case "/api2/json/nodes/pve01/storage/local-zfs/content":
			_, _ = w.Write([]byte(`{"data":[{"volid":"local-zfs:z-vol","content":"rootdir"},{"volid":"local-zfs:a-backup","content":"backup"}]}`))
		case "/api2/json/nodes/pve01/tasks", "/api2/json/cluster/ha/resources", "/api2/json/cluster/backup":
			_, _ = w.Write([]byte(`{"data":[]}`))
		case "/api2/json/cluster/ceph/status", "/api2/json/cluster/ceph/osd":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"errors":{"node":"no ceph"}}`))
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

	sut := newTestAPIServer(t)
	details, err := sut.loadProxmoxAssetDetails(
		context.Background(),
		"proxmox-storage-local-zfs",
		proxmoxSessionTarget{Kind: "storage", Node: "pve01", StorageName: "local-zfs"},
		proxmoxpkg.NewProxmoxRuntimeWithCollector(client, "collector-1"),
	)
	if err != nil {
		t.Fatalf("loadProxmoxAssetDetails storage branch failed: %v", err)
	}
	if details.Config["total"] != float64(1000) {
		t.Fatalf("expected storage status config payload, got %+v", details.Config)
	}
	if len(details.StorageContent) != 2 || details.StorageContent[0].Content != "backup" || details.StorageContent[0].VolID != "local-zfs:a-backup" {
		t.Fatalf("expected sorted storage content, got %+v", details.StorageContent)
	}
}

func TestLoadProxmoxAssetDetailsLXCWarningsAndCephData(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/version":
			_, _ = w.Write([]byte(`{"data":{"release":"8.4"}}`))
		case "/api2/json/nodes/pve01/lxc/200/config":
			_, _ = w.Write([]byte(`{"data":{"hostname":"ct-200","cores":2}}`))
		case "/api2/json/nodes/pve01/lxc/200/snapshot":
			_, _ = w.Write([]byte(`{"data":[{"name":"snap-1","snaptime":20}]}`))
		case "/api2/json/nodes/pve01/tasks":
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"errors":"tasks failed"}`))
		case "/api2/json/cluster/ha/resources":
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"errors":"ha failed"}`))
		case "/api2/json/nodes/pve01/lxc/200/firewall/rules":
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"errors":"firewall failed"}`))
		case "/api2/json/cluster/backup":
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"errors":"backup failed"}`))
		case "/api2/json/cluster/ceph/status":
			_, _ = w.Write([]byte(`{"data":{"health":{"status":"HEALTH_WARN"}}}`))
		case "/api2/json/cluster/ceph/osd":
			_, _ = w.Write([]byte(`{"data":[{"name":"osd.0","in":1,"up":1}]}`))
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

	sut := newTestAPIServer(t)
	details, err := sut.loadProxmoxAssetDetails(
		context.Background(),
		"proxmox-ct-200",
		proxmoxSessionTarget{Kind: "lxc", Node: "pve01", VMID: "200"},
		proxmoxpkg.NewProxmoxRuntimeWithCollector(client, "collector-1"),
	)
	if err != nil {
		t.Fatalf("loadProxmoxAssetDetails lxc branch failed: %v", err)
	}
	if len(details.Snapshots) != 1 || details.Snapshots[0].Name != "snap-1" {
		t.Fatalf("expected lxc snapshots to be loaded, got %+v", details.Snapshots)
	}
	if details.CephStatus == nil {
		t.Fatalf("expected ceph status to be included")
	}
	if len(details.CephOSDs) != 1 || details.CephOSDs[0].Name != "osd.0" {
		t.Fatalf("expected ceph osds to be included, got %+v", details.CephOSDs)
	}
	joinedWarnings := strings.Join(details.Warnings, " | ")
	for _, needle := range []string{"tasks unavailable", "ha resources unavailable", "firewall rules unavailable", "backup schedules unavailable"} {
		if !strings.Contains(joinedWarnings, needle) {
			t.Fatalf("expected warning %q in %q", needle, joinedWarnings)
		}
	}
}

func TestLoadProxmoxAssetDetailsNodeZFSWarningAndSuccess(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)

	makeClient := func(t *testing.T, handler http.HandlerFunc) *proxmox.Client {
		t.Helper()
		server := httptest.NewServer(handler)
		t.Cleanup(server.Close)
		client, err := proxmox.NewClient(proxmox.Config{
			BaseURL:     server.URL,
			TokenID:     "id",
			TokenSecret: "secret",
			Timeout:     5 * time.Second,
		})
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}
		return client
	}

	t.Run("zfs warning", func(t *testing.T) {
		client := makeClient(t, func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api2/json/version":
				_, _ = w.Write([]byte(`{"data":{"release":"8.3"}}`))
			case "/api2/json/nodes/pve01/status":
				_, _ = w.Write([]byte(`{"data":{"status":"online"}}`))
			case "/api2/json/nodes/pve01/tasks", "/api2/json/cluster/ha/resources", "/api2/json/nodes/pve01/firewall/rules", "/api2/json/cluster/backup":
				_, _ = w.Write([]byte(`{"data":[]}`))
			case "/api2/json/cluster/ceph/status", "/api2/json/cluster/ceph/osd":
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"errors":{"node":"no ceph"}}`))
			case "/api2/json/nodes/pve01/disks/zfs":
				w.WriteHeader(http.StatusBadGateway)
				_, _ = w.Write([]byte(`{"errors":"zfs failed"}`))
			default:
				t.Fatalf("unexpected proxmox path: %s", r.URL.Path)
			}
		})

		sut := newTestAPIServer(t)
		details, err := sut.loadProxmoxAssetDetails(
			context.Background(),
			"proxmox-node-pve01",
			proxmoxSessionTarget{Kind: "node", Node: "pve01"},
			proxmoxpkg.NewProxmoxRuntimeWithCollector(client, "collector-1"),
		)
		if err != nil {
			t.Fatalf("loadProxmoxAssetDetails node warning branch failed: %v", err)
		}
		if !strings.Contains(strings.Join(details.Warnings, " | "), "zfs pools unavailable") {
			t.Fatalf("expected zfs warning, got %+v", details.Warnings)
		}
	})

	t.Run("zfs success and unknown kind warning", func(t *testing.T) {
		client := makeClient(t, func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api2/json/version":
				_, _ = w.Write([]byte(`{"data":{"release":"8.3"}}`))
			case "/api2/json/nodes/pve01/status":
				_, _ = w.Write([]byte(`{"data":{"status":"online"}}`))
			case "/api2/json/nodes/pve01/tasks", "/api2/json/cluster/ha/resources", "/api2/json/cluster/backup":
				_, _ = w.Write([]byte(`{"data":[]}`))
			case "/api2/json/nodes/pve01/firewall/rules":
				_, _ = w.Write([]byte(`{"data":[]}`))
			case "/api2/json/cluster/ceph/status", "/api2/json/cluster/ceph/osd":
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"errors":{"node":"no ceph"}}`))
			case "/api2/json/nodes/pve01/disks/zfs":
				_, _ = w.Write([]byte(`{"data":[{"name":"pool-a","size":1000,"alloc":100,"free":900,"health":"ONLINE"}]}`))
			default:
				t.Fatalf("unexpected proxmox path: %s", r.URL.Path)
			}
		})

		sut := newTestAPIServer(t)
		details, err := sut.loadProxmoxAssetDetails(
			context.Background(),
			"proxmox-node-pve01",
			proxmoxSessionTarget{Kind: "node", Node: "pve01"},
			proxmoxpkg.NewProxmoxRuntimeWithCollector(client, "collector-1"),
		)
		if err != nil {
			t.Fatalf("loadProxmoxAssetDetails node success branch failed: %v", err)
		}
		if len(details.ZFSPools) != 1 || details.ZFSPools[0].Name != "pool-a" {
			t.Fatalf("expected zfs pools to be included, got %+v", details.ZFSPools)
		}

		unknownDetails, err := sut.loadProxmoxAssetDetails(
			context.Background(),
			"proxmox-weird",
			proxmoxSessionTarget{Kind: "weird", Node: "pve01"},
			proxmoxpkg.NewProxmoxRuntimeWithCollector(client, "collector-1"),
		)
		if err != nil {
			t.Fatalf("loadProxmoxAssetDetails unknown kind branch failed: %v", err)
		}
		if !strings.Contains(strings.Join(unknownDetails.Warnings, " | "), "unsupported proxmox kind") {
			t.Fatalf("expected unsupported-kind warning, got %+v", unknownDetails.Warnings)
		}
	})
}

func TestLoadProxmoxAssetDetailsStorageContentErrorAndTieSort(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/version":
			_, _ = w.Write([]byte(`{"data":{"release":"8.3"}}`))
		case "/api2/json/nodes/pve01/storage/local-zfs/status":
			_, _ = w.Write([]byte(`{"data":{"total":1000,"used":300,"avail":700}}`))
		case "/api2/json/nodes/pve01/storage/local-zfs/content":
			_, _ = w.Write([]byte(`{"data":[{"volid":"local-zfs:z-vol","content":"images"},{"volid":"local-zfs:a-vol","content":"images"}]}`))
		case "/api2/json/nodes/pve01/tasks", "/api2/json/cluster/ha/resources", "/api2/json/cluster/backup":
			_, _ = w.Write([]byte(`{"data":[]}`))
		case "/api2/json/cluster/ceph/status", "/api2/json/cluster/ceph/osd":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"errors":{"node":"no ceph"}}`))
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

	sut := newTestAPIServer(t)
	details, err := sut.loadProxmoxAssetDetails(
		context.Background(),
		"proxmox-storage-local-zfs",
		proxmoxSessionTarget{Kind: "storage", Node: "pve01", StorageName: "local-zfs"},
		proxmoxpkg.NewProxmoxRuntimeWithCollector(client, "collector-1"),
	)
	if err != nil {
		t.Fatalf("loadProxmoxAssetDetails storage tie-sort failed: %v", err)
	}
	if len(details.StorageContent) != 2 || details.StorageContent[0].VolID != "local-zfs:a-vol" {
		t.Fatalf("expected volid tie sort for same content type, got %+v", details.StorageContent)
	}

	errorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/version":
			_, _ = w.Write([]byte(`{"data":{"release":"8.3"}}`))
		case "/api2/json/nodes/pve01/storage/local-zfs/status":
			_, _ = w.Write([]byte(`{"data":{"total":1000,"used":300,"avail":700}}`))
		case "/api2/json/nodes/pve01/storage/local-zfs/content":
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"errors":"content failed"}`))
		case "/api2/json/nodes/pve01/tasks", "/api2/json/cluster/ha/resources", "/api2/json/cluster/backup":
			_, _ = w.Write([]byte(`{"data":[]}`))
		case "/api2/json/cluster/ceph/status", "/api2/json/cluster/ceph/osd":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"errors":{"node":"no ceph"}}`))
		default:
			t.Fatalf("unexpected proxmox path: %s", r.URL.Path)
		}
	}))
	defer errorServer.Close()

	errorClient, err := proxmox.NewClient(proxmox.Config{
		BaseURL:     errorServer.URL,
		TokenID:     "id",
		TokenSecret: "secret",
		Timeout:     5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	errorDetails, err := sut.loadProxmoxAssetDetails(
		context.Background(),
		"proxmox-storage-local-zfs",
		proxmoxSessionTarget{Kind: "storage", Node: "pve01", StorageName: "local-zfs"},
		proxmoxpkg.NewProxmoxRuntimeWithCollector(errorClient, "collector-1"),
	)
	if err != nil {
		t.Fatalf("loadProxmoxAssetDetails storage content warning branch failed: %v", err)
	}
	if !strings.Contains(strings.Join(errorDetails.Warnings, " | "), "storage content unavailable") {
		t.Fatalf("expected storage content warning, got %+v", errorDetails.Warnings)
	}
}

func TestLoadProxmoxAssetDetailsSnapshotWarningBranch(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/version":
			_, _ = w.Write([]byte(`{"data":{"release":"8.3"}}`))
		case "/api2/json/nodes/pve01/qemu/101/config":
			_, _ = w.Write([]byte(`{"data":{"name":"vm-101"}}`))
		case "/api2/json/nodes/pve01/qemu/101/snapshot":
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"errors":"snapshots failed"}`))
		case "/api2/json/nodes/pve01/tasks", "/api2/json/cluster/ha/resources", "/api2/json/nodes/pve01/qemu/101/firewall/rules", "/api2/json/cluster/backup":
			_, _ = w.Write([]byte(`{"data":[]}`))
		case "/api2/json/cluster/ceph/status", "/api2/json/cluster/ceph/osd":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"errors":{"node":"no ceph"}}`))
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

	sut := newTestAPIServer(t)
	details, err := sut.loadProxmoxAssetDetails(
		context.Background(),
		"proxmox-vm-101",
		proxmoxSessionTarget{Kind: "qemu", Node: "pve01", VMID: "101"},
		proxmoxpkg.NewProxmoxRuntimeWithCollector(client, "collector-1"),
	)
	if err != nil {
		t.Fatalf("loadProxmoxAssetDetails snapshot-warning branch failed: %v", err)
	}
	if !strings.Contains(strings.Join(details.Warnings, " | "), "snapshots unavailable") {
		t.Fatalf("expected snapshots warning, got %+v", details.Warnings)
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

func TestFilterAndSortProxmoxTasksLimitAndTieBreak(t *testing.T) {
	tasks := []proxmox.Task{
		{UPID: "UPID:pve01:...:100:", Node: "pve01", ID: "100", StartTime: 200},
		{UPID: "UPID:pve01:...:101:", Node: "pve01", ID: "101", StartTime: 200},
		{UPID: "UPID:pve01:...:102:", Node: "", ID: "102", StartTime: 210},
		{UPID: "UPID:pve02:...:103:", Node: "pve02", ID: "103", StartTime: 300},
	}

	filtered := filterAndSortProxmoxTasks(tasks, "pve01", "", 2)
	if len(filtered) != 2 {
		t.Fatalf("expected limit to reduce results to 2, got %d", len(filtered))
	}
	if filtered[0].ID != "102" {
		t.Fatalf("expected task with empty node and highest time to remain first, got %+v", filtered[0])
	}
	if filtered[1].UPID <= "UPID:pve01:...:100:" {
		t.Fatalf("expected tiebreak sort to keep lexicographically larger UPID first, got %+v", filtered[1])
	}
}

func TestProxmoxTaskMatchesVMIDVariants(t *testing.T) {
	if !proxmoxpkg.ProxmoxTaskMatchesVMID(proxmox.Task{ID: "101"}, "00101") {
		t.Fatalf("expected normalized numeric vmid match to succeed")
	}
	if !proxmoxpkg.ProxmoxTaskMatchesVMID(proxmox.Task{ID: "qemu/101"}, "101") {
		t.Fatalf("expected vmid match in composite task id")
	}
	if !proxmoxpkg.ProxmoxTaskMatchesVMID(proxmox.Task{UPID: "UPID:pve01:...:vzdump:101:root@pam:"}, "101") {
		t.Fatalf("expected vmid match in upid")
	}
	if proxmoxpkg.ProxmoxTaskMatchesVMID(proxmox.Task{ID: "102", UPID: "UPID:pve01:...:102:"}, "101") {
		t.Fatalf("did not expect mismatched vmid to match")
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

func TestMedianAndStdDevHelpers(t *testing.T) {
	if got := proxmoxpkg.MedianFloat64([]float64{3, 1, 2, 4}); got != 2.5 {
		t.Fatalf("expected even-length median 2.5, got %v", got)
	}
	if got := proxmoxpkg.MedianFloat64([]float64{3, 1, 2}); got != 2 {
		t.Fatalf("expected odd-length median 2, got %v", got)
	}
	if got := proxmoxpkg.MedianFloat64(nil); got != 0 {
		t.Fatalf("expected median nil fallback 0, got %v", got)
	}
	if got := proxmoxpkg.StdDevFloat64([]float64{2}); got != 0 {
		t.Fatalf("expected stddev single-sample fallback 0, got %v", got)
	}
	if got := proxmoxpkg.StdDevFloat64([]float64{2, 2, 2}); got != 0 {
		t.Fatalf("expected zero stddev for equal values, got %v", got)
	}
	if got := clampPercent(-5); got != 0 {
		t.Fatalf("expected clampPercent below 0 => 0, got %v", got)
	}
	if got := clampPercent(150); got != 100 {
		t.Fatalf("expected clampPercent above 100 => 100, got %v", got)
	}
	if got := parseStorageInsightsWindow("0d"); got != 7*24*time.Hour {
		t.Fatalf("expected invalid zero-day window fallback, got %s", got)
	}
	if got := parseStorageInsightsWindow(""); got != 7*24*time.Hour {
		t.Fatalf("expected empty window fallback, got %s", got)
	}
	if got := parseStorageInsightsWindow("31d"); got != 7*24*time.Hour {
		t.Fatalf("expected over-max window fallback, got %s", got)
	}
	if got := parseStorageInsightsWindow("xd"); got != 7*24*time.Hour {
		t.Fatalf("expected non-numeric day window fallback, got %s", got)
	}
	if got := parseStorageInsightsWindow("badx"); got != 7*24*time.Hour {
		t.Fatalf("expected invalid non-day duration fallback, got %s", got)
	}
	if got := parseStorageInsightsWindow("bad"); got != 7*24*time.Hour {
		t.Fatalf("expected invalid duration fallback, got %s", got)
	}
}

func TestSelectDiskSeriesPointsAndAnalyzeGrowthMediumConfidence(t *testing.T) {
	points := []telemetry.Point{
		{TS: 10, Value: 30},
		{TS: 20, Value: 31},
	}
	selected := proxmoxpkg.SelectDiskSeriesPoints([]telemetry.Series{
		{Metric: telemetry.MetricCPUUsedPercent, Points: []telemetry.Point{{TS: 1, Value: 10}}},
		{Metric: telemetry.MetricDiskUsedPercent, Points: points},
	})
	if len(selected) != len(points) {
		t.Fatalf("expected disk series to be selected, got %+v", selected)
	}
	if out := proxmoxpkg.SelectDiskSeriesPoints([]telemetry.Series{{Metric: telemetry.MetricCPUUsedPercent}}); out != nil {
		t.Fatalf("expected nil when disk metric is absent, got %+v", out)
	}

	mediumRate, mediumConfidence, _ := proxmoxpkg.AnalyzeDiskGrowth([]telemetry.Point{
		{TS: 1 * 24 * 60 * 60, Value: 50},
		{TS: 2 * 24 * 60 * 60, Value: 50.1},
		{TS: 3 * 24 * 60 * 60, Value: 50.2},
		{TS: 4 * 24 * 60 * 60, Value: 50.3},
		{TS: 5 * 24 * 60 * 60, Value: 50.4},
	})
	if mediumRate <= 0 || mediumConfidence != "medium" {
		t.Fatalf("expected medium-confidence growth branch, got rate=%v confidence=%s", mediumRate, mediumConfidence)
	}
}

func TestLoadProxmoxAssetDetailsVersionWarningBranch(t *testing.T) {
	allowInsecureTransportForConnectorTests(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/version":
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"errors":"version unavailable"}`))
		case "/api2/json/nodes/pve01/qemu/101/config":
			_, _ = w.Write([]byte(`{"data":{"name":"vm-101"}}`))
		case "/api2/json/nodes/pve01/qemu/101/snapshot",
			"/api2/json/nodes/pve01/tasks",
			"/api2/json/cluster/ha/resources",
			"/api2/json/nodes/pve01/qemu/101/firewall/rules",
			"/api2/json/cluster/backup":
			_, _ = w.Write([]byte(`{"data":[]}`))
		case "/api2/json/cluster/ceph/status", "/api2/json/cluster/ceph/osd":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"errors":{"node":"no ceph"}}`))
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

	sut := newTestAPIServer(t)
	details, err := sut.loadProxmoxAssetDetails(
		context.Background(),
		"proxmox-vm-101",
		proxmoxSessionTarget{Kind: "qemu", Node: "pve01", VMID: "101"},
		proxmoxpkg.NewProxmoxRuntimeWithCollector(client, "collector-1"),
	)
	if err != nil {
		t.Fatalf("loadProxmoxAssetDetails failed: %v", err)
	}
	if !strings.Contains(strings.Join(details.Warnings, " | "), "version unavailable") {
		t.Fatalf("expected version warning, got %+v", details.Warnings)
	}
}

func TestProxmoxStoragePoolStateAndNameEdgeBranches(t *testing.T) {
	states := proxmoxpkg.BuildProxmoxStoragePoolStates([]assets.Asset{
		{
			ID:     "empty-pool-name",
			Type:   "storage-pool",
			Source: "proxmox",
			Metadata: map[string]string{
				"storage_id": "storage/pve01/",
			},
		},
	}, proxmoxSessionTarget{Kind: "node", Node: "pve01"}, "")
	if len(states) != 0 {
		t.Fatalf("expected empty pool name asset to be skipped, got %+v", states)
	}

	if got := proxmoxpkg.ProxmoxStoragePoolNameFromAsset(assets.Asset{Name: "pool/"}); got != "pool/" {
		t.Fatalf("expected trailing slash fallback to return original name, got %q", got)
	}
}

func TestBuildProxmoxStorageInsightPoolAdditionalBranches(t *testing.T) {
	now := time.Now().UTC()

	t.Run("status fallback maxdisk and disk", func(t *testing.T) {
		lastScrub := now.Add(-2 * time.Hour).Unix()
		pool := proxmoxpkg.BuildProxmoxStorageInsightPool(proxmoxStoragePoolState{
			PoolName: "status-fallback",
			Status: map[string]any{
				"maxdisk":       int64(1000),
				"disk":          int64(640),
				"last_scrub":    lastScrub,
				"scrub_overdue": int64(1),
			},
		}, now)
		if pool.SizeBytes == nil || *pool.SizeBytes != 1000 {
			t.Fatalf("expected maxdisk fallback size=1000, got %+v", pool.SizeBytes)
		}
		if pool.UsedBytes == nil || *pool.UsedBytes != 640 {
			t.Fatalf("expected disk fallback used=640, got %+v", pool.UsedBytes)
		}
		if pool.Scrub.LastCompletedAt == "" {
			t.Fatalf("expected status last_scrub timestamp to populate scrub.last_completed_at")
		}
		if !pool.Scrub.Overdue {
			t.Fatalf("expected scrub_overdue status to set scrub.overdue")
		}
	})

	t.Run("metadata and disk series used percent fallbacks", func(t *testing.T) {
		metadataPool := proxmoxpkg.BuildProxmoxStorageInsightPool(proxmoxStoragePoolState{
			PoolName: "metadata-fallback",
			HasAsset: true,
			Asset: assets.Asset{
				Metadata: map[string]string{
					"disk_percent": "88",
				},
			},
		}, now)
		if metadataPool.UsedPercent == nil || *metadataPool.UsedPercent != 88 {
			t.Fatalf("expected metadata fallback used percent 88, got %+v", metadataPool.UsedPercent)
		}

		lastScrubPool := proxmoxpkg.BuildProxmoxStorageInsightPool(proxmoxStoragePoolState{
			PoolName: "metadata-last-scrub",
			HasAsset: true,
			Asset: assets.Asset{
				Metadata: map[string]string{
					"last_scrub": fmt.Sprintf("%d", now.Add(-24*time.Hour).Unix()),
				},
			},
		}, now)
		if lastScrubPool.Scrub.LastCompletedAt == "" {
			t.Fatalf("expected metadata last_scrub timestamp to populate scrub.last_completed_at")
		}

		seriesPool := proxmoxpkg.BuildProxmoxStorageInsightPool(proxmoxStoragePoolState{
			PoolName: "series-fallback",
			DiskSeries: []telemetry.Point{
				{TS: time.Now().Add(-2 * time.Hour).Unix(), Value: 71},
				{TS: time.Now().Add(-time.Hour).Unix(), Value: 73},
			},
			Content: []proxmox.StorageContent{
				{VolID: "series:ignored", Content: "images", VMID: 0},
				{VolID: "series:subvol-201-disk-0", Content: "rootdir", VMID: 201},
			},
		}, now)
		if seriesPool.UsedPercent == nil || *seriesPool.UsedPercent != 73 {
			t.Fatalf("expected disk-series fallback used percent 73, got %+v", seriesPool.UsedPercent)
		}
		if seriesPool.DependentWorkloads.CTCount != 1 || seriesPool.DependentWorkloads.VMCount != 0 {
			t.Fatalf("expected VMID<=0 to be skipped and CT mapping to remain, got %+v", seriesPool.DependentWorkloads)
		}
	})
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

func TestAnalyzeGrowthAndRiskAdditionalBranches(t *testing.T) {
	rate, confidence, latestTS := proxmoxpkg.AnalyzeDiskGrowth([]telemetry.Point{
		{TS: 1000, Value: 60},
		{TS: 1000, Value: 65},
	})
	if rate != 0 || confidence != "low" || latestTS != 1000 {
		t.Fatalf("expected zero-rate low-confidence for non-increasing timestamps, got rate=%v confidence=%s latest=%d", rate, confidence, latestTS)
	}

	invalidRate, invalidConfidence, invalidLatest := proxmoxpkg.AnalyzeDiskGrowth([]telemetry.Point{
		{TS: 1000, Value: math.NaN()},
		{TS: 2000, Value: math.NaN()},
	})
	if invalidRate != 0 || invalidConfidence != "low" || invalidLatest != 2000 {
		t.Fatalf("expected NaN rate samples to be skipped, got rate=%v confidence=%s latest=%d", invalidRate, invalidConfidence, invalidLatest)
	}

	used := 80.0
	daysToFull := 80.0
	score, state, reasons := proxmoxpkg.ComputeStorageRisk(proxmoxpkg.ProxmoxStorageInsightPool{
		Health:      "ONLINE",
		UsedPercent: &used,
		Forecast: proxmoxpkg.ProxmoxStorageForecast{
			DaysToFull: &daysToFull,
		},
	})
	if score != 26 || state != "watch" {
		t.Fatalf("expected watch state with score 26, got score=%d state=%s reasons=%v", score, state, reasons)
	}
}

func TestProxmoxTaskAndNodeHandlersAdditionalErrorBranches(t *testing.T) {
	sut := newTestAPIServer(t)

	req := httptest.NewRequest(http.MethodGet, "/proxmox/tasks/", nil)
	rec := httptest.NewRecorder()
	sut.handleProxmoxTaskLog(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing task-log path, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/proxmox/tasks/", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "owner", "owner"))
	rec = httptest.NewRecorder()
	sut.handleProxmoxTaskStop(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing task-stop path, got %d", rec.Code)
	}

	errorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/nodes/pve01/tasks/UPID-1/log":
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"errors":"task log unavailable"}`))
		case "/api2/json/nodes/pve01/tasks/UPID-1":
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"errors":"stop failed"}`))
		case "/api2/json/cluster/status":
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"errors":"cluster unavailable"}`))
		case "/api2/json/nodes/pve01/network":
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"errors":"network unavailable"}`))
		default:
			t.Fatalf("unexpected proxmox path: %s", r.URL.Path)
		}
	}))
	defer errorServer.Close()
	configureSingleProxmoxCollector(t, sut, errorServer.URL, "collector-proxmox-1")

	req = httptest.NewRequest(http.MethodGet, "/proxmox/tasks/pve01/UPID-1/log", nil)
	rec = httptest.NewRecorder()
	sut.handleProxmoxTaskLog(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 when proxmox task-log fetch fails, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/proxmox/tasks/pve01/UPID-1/stop", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "owner", "owner"))
	rec = httptest.NewRecorder()
	sut.handleProxmoxTaskStop(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 when proxmox task stop fails, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/proxmox/cluster/status", nil)
	rec = httptest.NewRecorder()
	sut.handleProxmoxClusterStatus(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 when cluster status fetch fails, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/proxmox/nodes/pve01/network", nil)
	rec = httptest.NewRecorder()
	sut.handleProxmoxNodeNetwork(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 when node network fetch fails, got %d", rec.Code)
	}
}

func TestProxmoxSnapshotTaskAndHAEdgeBranches(t *testing.T) {
	snapshots := sortProxmoxSnapshots([]proxmox.Snapshot{
		{Name: "z-snap", SnapTime: 50},
		{Name: "a-snap", SnapTime: 50},
	})
	if len(snapshots) != 2 || snapshots[0].Name != "a-snap" {
		t.Fatalf("expected same-time snapshot tie sort by name, got %+v", snapshots)
	}

	if !proxmoxpkg.ProxmoxTaskMatchesVMID(proxmox.Task{ID: "any"}, "   ") {
		t.Fatalf("expected empty VMID filter to match all tasks")
	}

	match, related := selectProxmoxHA([]proxmox.HAResource{
		{SID: "ct:202", Node: "pve01", State: "started"},
		{SID: "vm:202", Node: "pve01", State: "started"},
	}, proxmoxSessionTarget{Kind: "lxc", VMID: "202"})
	if match == nil || match.SID != "ct:202" || len(related) != 1 {
		t.Fatalf("expected LXC HA SID ct:202 selection, got match=%+v related=%+v", match, related)
	}
}

func configureDualProxmoxCollectors(t *testing.T, sut *apiServer, collectorOneURL, collectorTwoURL string) {
	t.Helper()

	createProxmoxCredentialProfile(
		t,
		sut,
		"cred-proxmox-collector-1",
		"labtether@pve!collector1",
		"token-secret-1",
		collectorOneURL,
	)
	createProxmoxCredentialProfile(
		t,
		sut,
		"cred-proxmox-collector-2",
		"labtether@pve!collector2",
		"token-secret-2",
		collectorTwoURL,
	)

	sut.hubCollectorStore = &stubHubCollectorStore{
		collectors: []hubcollector.Collector{
			{
				ID:            "collector-proxmox-1",
				AssetID:       "proxmox-cluster-one",
				CollectorType: hubcollector.CollectorTypeProxmox,
				Enabled:       true,
				Config: map[string]any{
					"base_url":      collectorOneURL,
					"token_id":      "labtether@pve!collector1",
					"credential_id": "cred-proxmox-collector-1",
					"skip_verify":   true,
				},
			},
			{
				ID:            "collector-proxmox-2",
				AssetID:       "proxmox-cluster-two",
				CollectorType: hubcollector.CollectorTypeProxmox,
				Enabled:       true,
				Config: map[string]any{
					"base_url":      collectorTwoURL,
					"token_id":      "labtether@pve!collector2",
					"credential_id": "cred-proxmox-collector-2",
					"skip_verify":   true,
				},
			},
		},
	}
}

func configureSingleProxmoxCollector(t *testing.T, sut *apiServer, collectorURL, collectorID string) {
	t.Helper()

	credentialID := "cred-" + collectorID
	createProxmoxCredentialProfile(
		t,
		sut,
		credentialID,
		"labtether@pve!agent",
		"token-secret",
		collectorURL,
	)

	sut.hubCollectorStore = &stubHubCollectorStore{
		collectors: []hubcollector.Collector{
			{
				ID:            collectorID,
				AssetID:       "proxmox-cluster-test",
				CollectorType: hubcollector.CollectorTypeProxmox,
				Enabled:       true,
				Config: map[string]any{
					"base_url":      collectorURL,
					"token_id":      "labtether@pve!agent",
					"credential_id": credentialID,
					"skip_verify":   true,
				},
			},
		},
	}
}

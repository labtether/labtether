package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/connectors/docker"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/runtimesettings"
	"github.com/labtether/labtether/internal/synthetic"
	"github.com/labtether/labtether/internal/telemetry"
	"github.com/labtether/labtether/internal/telemetry/promexport"
)

func TestMetricsExportStartupRegistersEveryBoundedProductionBridge(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()
	sut.dockerCoordinator = docker.NewCoordinator(sut.agentMgr)
	initMetricsExport(sut, nil)
	names := make(map[string]bool)
	for _, name := range sut.bridgeRegistry.Names() {
		names[name] = true
	}
	for _, required := range []string{
		"docker-stats", "alert-state", "agent-presence", "process-metrics",
		"network-interfaces", "disk-mounts", "service-health", "synthetic-checks",
		"site-reliability",
	} {
		if !names[required] {
			t.Errorf("production bridge %q was not registered: %v", required, sut.bridgeRegistry.Names())
		}
	}
	for _, removedDeadBridge := range []string{"proxmox-metrics", "pbs-metrics"} {
		if names[removedDeadBridge] {
			t.Errorf("dead duplicate polling bridge %q remains registered", removedDeadBridge)
		}
	}
}

func TestProcessMetricsRequireDynamicOptInAndEnforceTopN(t *testing.T) {
	t.Setenv("LABTETHER_PROCESS_METRICS_ENABLED", "false")
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()
	serverConn, clientConn, cleanup := createWSPairForNetworkTest(t)
	defer cleanup()
	sut.agentMgr.Register(agentmgr.NewAgentConn(serverConn, "node-process-metrics", "linux"))
	defer sut.agentMgr.Unregister("node-process-metrics")

	adapter := newAgentTelemetryAdapter(sut)
	if got := adapter.AllProcessMetrics(); got != nil {
		t.Fatalf("default-disabled process collection returned %+v", got)
	}
	if _, err := sut.runtimeStore.SaveRuntimeSettingOverrides(map[string]string{
		runtimesettings.KeyProcessMetricsEnabled: "true",
		runtimesettings.KeyProcessMetricsTopN:    "2",
	}); err != nil {
		t.Fatalf("enable process metrics: %v", err)
	}

	respond := func(wantLimit int) <-chan struct{} {
		done := make(chan struct{})
		go func() {
			defer close(done)
			var outbound agentmgr.Message
			if err := clientConn.ReadJSON(&outbound); err != nil {
				t.Errorf("read process telemetry request: %v", err)
				return
			}
			var request agentmgr.ProcessListData
			if err := json.Unmarshal(outbound.Data, &request); err != nil {
				t.Errorf("decode process telemetry request: %v", err)
				return
			}
			if outbound.Type != agentmgr.MsgProcessList || request.SortBy != "cpu" || request.Limit != wantLimit {
				t.Errorf("unexpected process telemetry request: message=%+v request=%+v", outbound, request)
				return
			}
			payload, _ := json.Marshal(agentmgr.ProcessListedData{RequestID: request.RequestID, Processes: []agentmgr.ProcessInfo{
				{PID: 40, Name: "invalid", CPUPct: 20001, MemPct: 1, MemRSS: 40},
				{PID: 30, Name: "third", CPUPct: 3, MemPct: 1, MemRSS: 30},
				{PID: 10, Name: "first", CPUPct: 30, MemPct: 3, MemRSS: 10},
				{PID: 20, Name: "second", CPUPct: 20, MemPct: 2, MemRSS: 20},
			}})
			sut.processAgentProcessListed(&agentmgr.AgentConn{AssetID: "node-process-metrics"}, agentmgr.Message{Type: agentmgr.MsgProcessListed, Data: payload})
		}()
		return done
	}

	done := respond(2)
	entries := adapter.AllProcessMetrics()
	<-done
	if len(entries) != 2 || entries[0].Labels["process_pid"] != "10" || entries[1].Labels["process_pid"] != "20" {
		t.Fatalf("top-2 process metrics = %+v", entries)
	}
	for _, entry := range entries {
		if len(entry.Labels) != 2 || entry.Labels["process_name"] == "" {
			t.Fatalf("process labels include unbounded fields: %+v", entry.Labels)
		}
	}

	if _, err := sut.runtimeStore.SaveRuntimeSettingOverrides(map[string]string{
		runtimesettings.KeyProcessMetricsTopN: "1",
	}); err != nil {
		t.Fatalf("change process top-N: %v", err)
	}
	done = respond(1)
	entries = adapter.AllProcessMetrics()
	<-done
	if len(entries) != 1 || entries[0].Labels["process_pid"] != "10" {
		t.Fatalf("dynamic top-1 process metrics = %+v", entries)
	}
}

func TestCollectAgentAssetsUsesSharedBoundAndContainsCollectorPanic(t *testing.T) {
	assetIDs := []string{"one", "two", "panic", "three", "four", "five"}
	sem := make(chan struct{}, 2)
	var current atomic.Int32
	var maximum atomic.Int32
	entries := collectAgentAssets(assetIDs, sem, func(assetID string) []string {
		active := current.Add(1)
		defer current.Add(-1)
		for {
			prior := maximum.Load()
			if active <= prior || maximum.CompareAndSwap(prior, active) {
				break
			}
		}
		if assetID == "panic" {
			panic("collector panic must remain contained")
		}
		time.Sleep(time.Millisecond)
		return []string{assetID}
	})
	if len(entries) != len(assetIDs)-1 {
		t.Fatalf("collected entries after contained panic = %v", entries)
	}
	if got := maximum.Load(); got > 2 {
		t.Fatalf("shared request concurrency = %d, want <= 2", got)
	}
}

func TestNetworkMetricsDeriveRatesFromCumulativeCounters(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()
	serverConn, clientConn, cleanup := createWSPairForNetworkTest(t)
	defer cleanup()
	sut.agentMgr.Register(agentmgr.NewAgentConn(serverConn, "node-network-metrics", "linux"))
	defer sut.agentMgr.Unregister("node-network-metrics")
	adapter := newAgentTelemetryAdapter(sut)

	respond := func(rx, tx, rxPackets, txPackets uint64) <-chan struct{} {
		done := make(chan struct{})
		go func() {
			defer close(done)
			var outbound agentmgr.Message
			if err := clientConn.ReadJSON(&outbound); err != nil {
				t.Errorf("read network telemetry request: %v", err)
				return
			}
			var request agentmgr.NetworkListData
			_ = json.Unmarshal(outbound.Data, &request)
			payload, _ := json.Marshal(agentmgr.NetworkListedData{RequestID: request.RequestID, Interfaces: []agentmgr.NetInterface{{
				Name: "eth0", RXBytes: rx, TXBytes: tx, RXPackets: rxPackets, TXPackets: txPackets,
			}}})
			sut.processAgentNetworkListed(&agentmgr.AgentConn{AssetID: "node-network-metrics"}, agentmgr.Message{Type: agentmgr.MsgNetworkListed, Data: payload})
		}()
		return done
	}

	done := respond(100, 200, 10, 20)
	if first := adapter.AllNetworkInterfaces(); len(first) != 0 {
		t.Fatalf("first cumulative network sample fabricated a rate: %+v", first)
	}
	<-done
	time.Sleep(time.Millisecond)
	done = respond(160, 260, 16, 26)
	second := adapter.AllNetworkInterfaces()
	<-done
	if len(second) != 1 || second[0].RXBytes <= 0 || second[0].TXBytes <= 0 || second[0].RXPackets != 16 || second[0].TXPackets != 26 {
		t.Fatalf("derived network metrics = %+v", second)
	}
}

func TestDiskMetricsUseBoundedValidatedMountData(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()
	serverConn, clientConn, cleanup := createWSPairForNetworkTest(t)
	defer cleanup()
	sut.agentMgr.Register(agentmgr.NewAgentConn(serverConn, "node-disk-metrics", "linux"))
	defer sut.agentMgr.Unregister("node-disk-metrics")
	adapter := newAgentTelemetryAdapter(sut)

	done := make(chan struct{})
	go func() {
		defer close(done)
		var outbound agentmgr.Message
		if err := clientConn.ReadJSON(&outbound); err != nil {
			t.Errorf("read disk telemetry request: %v", err)
			return
		}
		var request agentmgr.DiskListData
		_ = json.Unmarshal(outbound.Data, &request)
		payload, _ := json.Marshal(agentmgr.DiskListedData{RequestID: request.RequestID, Mounts: []agentmgr.MountInfo{
			{MountPoint: "/", Total: 100, Used: 40, Available: 60, UsePct: 40},
			{MountPoint: "/invalid", Total: 100, Used: 60, Available: 60, UsePct: 60},
		}})
		sut.processAgentDiskListed(&agentmgr.AgentConn{AssetID: "node-disk-metrics"}, agentmgr.Message{Type: agentmgr.MsgDiskListed, Data: payload})
	}()
	entries := adapter.AllDiskMounts()
	<-done
	if len(entries) != 1 || entries[0].Labels["mount_point"] != "/" || entries[0].Used != 40 {
		t.Fatalf("validated disk metrics = %+v", entries)
	}
}

func TestServiceHealthMetricsUseStableIDWithoutURLLabel(t *testing.T) {
	sut := newTestAPIServer(t)
	report, _ := json.Marshal(agentmgr.WebServiceReportData{
		HostAssetID: "node-service-metrics",
		Services: []agentmgr.DiscoveredWebService{{
			ID: "svc-private", Name: "Private API",
			URL:    "https://user:password@example.test/?token=secret",
			Status: "up", ResponseMs: 25, HostAssetID: "node-service-metrics",
		}},
	})
	sut.webServiceCoordinator.HandleReport("node-service-metrics", agentmgr.Message{Type: agentmgr.MsgWebServiceReport, Data: report})
	entries := (&serviceHealthMetricsAdapter{coordinator: sut.webServiceCoordinator}).AllServiceHealth()
	if len(entries) != 1 || entries[0].Labels["target"] != "svc-private" || entries[0].Labels["service_name"] != "Private API" {
		t.Fatalf("service health metrics = %+v", entries)
	}
	for _, forbidden := range []string{"service_url", "url"} {
		if _, leaked := entries[0].Labels[forbidden]; leaked {
			t.Fatalf("service URL leaked through %q label: %+v", forbidden, entries[0].Labels)
		}
	}
}

func TestSyntheticMetricsUseHubScopeAndNeverExportTarget(t *testing.T) {
	store := persistence.NewMemorySyntheticStore()
	enabled := true
	check, err := store.CreateSyntheticCheck(synthetic.CreateCheckRequest{
		Name: "Private URL", CheckType: synthetic.CheckTypeHTTP,
		Target: "https://user:password@example.test/?token=secret", Enabled: &enabled,
	})
	if err != nil {
		t.Fatalf("create synthetic check: %v", err)
	}
	latency := 12
	recorded, err := store.RecordSyntheticResult(check.ID, synthetic.Result{Status: synthetic.ResultStatusOK, LatencyMS: &latency})
	if err != nil {
		t.Fatalf("record synthetic result: %v", err)
	}
	adapter := &syntheticMetricsAdapter{snapshotStore: store, lastResultByCheck: make(map[string]string)}
	entries := adapter.AllSyntheticCheckMetrics()
	if len(entries) != 1 || entries[0].Labels["check_id"] != check.ID || entries[0].CollectedAt != recorded.CheckedAt {
		t.Fatalf("synthetic metric entry = %+v", entries)
	}
	if _, leaked := entries[0].Labels["target"]; leaked {
		t.Fatalf("synthetic target leaked into labels: %+v", entries[0].Labels)
	}
	if repeated := adapter.AllSyntheticCheckMetrics(); len(repeated) != 0 {
		t.Fatalf("unchanged persisted result was re-emitted: %+v", repeated)
	}
}

func TestPrometheusSnapshotKeepsHubMetricsSeparateFromCollidingRealAssetID(t *testing.T) {
	assetStore := persistence.NewMemoryAssetStore()
	telemetryStore := persistence.NewMemoryTelemetryStore()
	now := time.Now().UTC()
	if _, err := assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "labtether-hub", Type: "server", Name: "Real Asset", Source: "test", Status: "online",
	}); err != nil {
		t.Fatalf("seed asset: %v", err)
	}
	if err := telemetryStore.AppendSamples(context.Background(), []telemetry.MetricSample{
		{AssetID: "labtether-hub", Metric: telemetry.MetricCPUUsedPercent, Unit: "percent", Value: 12, CollectedAt: now},
		{
			Scope: telemetry.MetricScopeHubAlerts, Metric: telemetry.MetricAlertEvaluationDurationMs,
			Unit: "ms", Value: 9, CollectedAt: now, Labels: map[string]string{"rule_id": "rule-agent-down", "rule_name": "agent-down"},
		},
	}); err != nil {
		t.Fatalf("seed telemetry: %v", err)
	}

	adapter := &prometheusSnapshotAdapter{
		telemetryStore: telemetryStore,
		hubMetricStore: telemetryStore,
		assetStore:     assetStore,
	}
	snapshots, metas := adapter.scrape()
	if len(snapshots["labtether-hub"]) != 1 {
		t.Fatalf("asset telemetry missing from scrape: %+v", snapshots)
	}
	if metas["labtether-hub"].Name != "Real Asset" {
		t.Fatalf("hub snapshot overwrote colliding real asset metadata: %+v", metas)
	}
	hub := adapter.hubScrape()[telemetry.MetricScopeHubAlerts]
	if len(hub) != 1 {
		t.Fatalf("hub telemetry sample count = %d, want 1", len(hub))
	}
	if hub[0].Metric != telemetry.MetricAlertEvaluationDurationMs || hub[0].Labels["rule_id"] != "rule-agent-down" || hub[0].Labels["rule_name"] != "agent-down" {
		t.Fatalf("hub telemetry labels were not preserved: %+v", hub[0])
	}
	if _, exists := metas[telemetry.MetricScopeHubAlerts]; exists {
		t.Fatalf("hub metrics must not create synthetic asset metadata: %+v", metas)
	}

	assetList, err := assetStore.ListAssets()
	if err != nil {
		t.Fatalf("list assets: %v", err)
	}
	if len(assetList) != 1 || assetList[0].ID != "labtether-hub" {
		t.Fatalf("hub metric created a user-visible asset: %+v", assetList)
	}
}

func TestPrometheusProductionAdapterPreservesLabeledSubseriesOverHTTP(t *testing.T) {
	assetStore := persistence.NewMemoryAssetStore()
	telemetryStore := persistence.NewMemoryTelemetryStore()
	if _, err := assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "labeled-asset", Type: "server", Name: "Labeled Asset", Source: "test", Status: "online",
	}); err != nil {
		t.Fatalf("seed asset: %v", err)
	}
	now := time.Now().UTC()
	samples := []telemetry.MetricSample{
		{AssetID: "labeled-asset", Metric: telemetry.MetricDiskUsedBytes, Unit: "bytes", Value: 10, CollectedAt: now, Labels: map[string]string{"mount_point": "/"}},
		{AssetID: "labeled-asset", Metric: telemetry.MetricDiskUsedBytes, Unit: "bytes", Value: 20, CollectedAt: now, Labels: map[string]string{"mount_point": "/data"}},
		{AssetID: "labeled-asset", Metric: telemetry.MetricInterfaceRXBytesPerSec, Unit: "bytes_per_sec", Value: 30, CollectedAt: now, Labels: map[string]string{"interface": "eth0"}},
		{AssetID: "labeled-asset", Metric: telemetry.MetricInterfaceRXBytesPerSec, Unit: "bytes_per_sec", Value: 40, CollectedAt: now, Labels: map[string]string{"interface": "wlan0"}},
		{AssetID: "labeled-asset", Metric: telemetry.MetricProcessCPUPercent, Unit: "percent", Value: 50, CollectedAt: now, Labels: map[string]string{"process_name": "alpha", "process_pid": "101"}},
		{AssetID: "labeled-asset", Metric: telemetry.MetricProcessCPUPercent, Unit: "percent", Value: 60, CollectedAt: now, Labels: map[string]string{"process_name": "beta", "process_pid": "202"}},
	}
	if err := telemetryStore.AppendSamples(context.Background(), samples); err != nil {
		t.Fatalf("seed labeled telemetry: %v", err)
	}
	source := &cachedSnapshotAdapter{
		inner: &prometheusSnapshotAdapter{
			telemetryStore: telemetryStore,
			assetStore:     assetStore,
		},
		cacheTTL: time.Second,
	}
	handler := promexport.NewHandler(source)
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("metrics status = %d, body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, expected := range []string{
		`mount_point="/"`, `mount_point="/data"`,
		`interface="eth0"`, `interface="wlan0"`,
		`process_name="alpha"`, `process_name="beta"`,
		`process_pid="101"`, `process_pid="202"`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("HTTP scrape missing labeled subseries %s:\n%s", expected, body)
		}
	}
	if got := strings.Count(body, "labtether_disk_used_bytes{"); got != 2 {
		t.Fatalf("disk family series count = %d, want 2", got)
	}
	if got := strings.Count(body, "labtether_interface_rx_bytes_per_sec{"); got != 2 {
		t.Fatalf("interface family series count = %d, want 2", got)
	}
	if got := strings.Count(body, "labtether_process_cpu_percent{"); got != 2 {
		t.Fatalf("process family series count = %d, want 2", got)
	}
}

func TestPrometheusProductionAdapterExportsAssetInfoWithoutFreshSamples(t *testing.T) {
	assetStore := persistence.NewMemoryAssetStore()
	telemetryStore := persistence.NewMemoryTelemetryStore()
	if _, err := assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "quiet-asset", Type: "server", Name: "Quiet Asset", Source: "test", Status: "online",
	}); err != nil {
		t.Fatalf("seed asset: %v", err)
	}
	source := &cachedSnapshotAdapter{
		inner: &prometheusSnapshotAdapter{
			telemetryStore: telemetryStore,
			assetStore:     assetStore,
		},
		cacheTTL: time.Second,
	}
	handler := promexport.NewHandler(source)
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("metrics status = %d, body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `labtether_asset_info{asset_id="quiet-asset",asset_name="Quiet Asset"`) {
		t.Fatalf("asset_info missing for asset without fresh samples:\n%s", body)
	}
}

type failingHubMetricStore struct {
	maxSeries   int
	hasDeadline bool
}

type failingLabeledMetricStore struct {
	maxSeries   int
	hasDeadline bool
	calls       int
}

func (f *failingLabeledMetricStore) LatestLabeledMetricSnapshots(ctx context.Context, _ []string, _ time.Time, maxSeries int) (map[string][]telemetry.MetricSample, error) {
	f.calls++
	f.maxSeries = maxSeries
	_, f.hasDeadline = ctx.Deadline()
	return nil, errors.New("forced labeled snapshot failure")
}

func TestPrometheusAssetSnapshotFailureIsDeadlineBoundAndFailClosed(t *testing.T) {
	assetStore := persistence.NewMemoryAssetStore()
	if _, err := assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "asset-1", Type: "server", Name: "Asset One", Source: "test", Status: "online",
	}); err != nil {
		t.Fatalf("seed asset: %v", err)
	}
	failingStore := &failingLabeledMetricStore{}
	adapter := &prometheusSnapshotAdapter{telemetryStore: failingStore, assetStore: assetStore}
	snapshots, metas := adapter.scrape()
	if snapshots != nil {
		t.Fatalf("failed labeled snapshot returned partial data: %+v", snapshots)
	}
	if metas != nil {
		t.Fatalf("failed labeled snapshot returned partial metadata: %+v", metas)
	}
	if !failingStore.hasDeadline || failingStore.maxSeries != telemetry.MaxPrometheusAssetMetricSeries {
		t.Fatalf("asset snapshot was not deadline/series bounded: deadline=%v max=%d", failingStore.hasDeadline, failingStore.maxSeries)
	}
}

type fixedAssetStore struct {
	assets []assets.Asset
}

func (f *fixedAssetStore) UpsertAssetHeartbeat(assets.HeartbeatRequest) (assets.Asset, error) {
	return assets.Asset{}, errors.New("not implemented")
}
func (f *fixedAssetStore) UpdateAsset(string, assets.UpdateRequest) (assets.Asset, error) {
	return assets.Asset{}, errors.New("not implemented")
}
func (f *fixedAssetStore) ListAssets() ([]assets.Asset, error) { return f.assets, nil }
func (f *fixedAssetStore) GetAsset(string) (assets.Asset, bool, error) {
	return assets.Asset{}, false, nil
}
func (f *fixedAssetStore) DeleteAsset(string) error { return errors.New("not implemented") }

func TestPrometheusAssetSnapshotRejectsOversizedInventoryBeforeStoreWork(t *testing.T) {
	failingStore := &failingLabeledMetricStore{}
	adapter := &prometheusSnapshotAdapter{
		telemetryStore: failingStore,
		assetStore: &fixedAssetStore{
			assets: make([]assets.Asset, telemetry.MaxPrometheusSnapshotAssets+1),
		},
	}
	snapshots, metas := adapter.scrape()
	if snapshots != nil || metas != nil {
		t.Fatalf("oversized inventory returned partial scrape: snapshots=%+v metas=%+v", snapshots, metas)
	}
	if failingStore.calls != 0 {
		t.Fatalf("oversized inventory reached telemetry snapshot store %d times", failingStore.calls)
	}
}

func (f *failingHubMetricStore) HubMetricSnapshots(ctx context.Context, _ time.Time, maxSeries int) (map[string][]telemetry.MetricSample, error) {
	f.maxSeries = maxSeries
	_, f.hasDeadline = ctx.Deadline()
	return nil, errors.New("forced hub snapshot failure")
}

func TestPrometheusHubSnapshotFailureIsBoundedAndDoesNotDropAssets(t *testing.T) {
	assetStore := persistence.NewMemoryAssetStore()
	telemetryStore := persistence.NewMemoryTelemetryStore()
	if _, err := assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "asset-1", Type: "server", Name: "Asset One", Source: "test", Status: "online",
	}); err != nil {
		t.Fatalf("seed asset: %v", err)
	}
	if err := telemetryStore.AppendSamples(context.Background(), []telemetry.MetricSample{{
		AssetID: "asset-1", Metric: telemetry.MetricCPUUsedPercent, Unit: "percent", Value: 12, CollectedAt: time.Now().UTC(),
	}}); err != nil {
		t.Fatalf("seed telemetry: %v", err)
	}
	failingStore := &failingHubMetricStore{}
	adapter := &prometheusSnapshotAdapter{
		telemetryStore: telemetryStore,
		hubMetricStore: failingStore,
		assetStore:     assetStore,
	}
	snapshots, metas := adapter.scrape()
	if len(snapshots["asset-1"]) != 1 || metas["asset-1"].Name != "Asset One" {
		t.Fatalf("hub failure affected asset scrape: snapshots=%+v metas=%+v", snapshots, metas)
	}
	if got := adapter.hubScrape(); got != nil {
		t.Fatalf("failed hub scrape = %+v, want nil fail-soft result", got)
	}
	if !failingStore.hasDeadline || failingStore.maxSeries != telemetry.MaxHubMetricSnapshotSeries {
		t.Fatalf("hub query was not deadline/row bounded: deadline=%v max=%d", failingStore.hasDeadline, failingStore.maxSeries)
	}
}

type recordingReliabilityMetricSnapshotStore struct {
	snapshots   []persistence.ReliabilityMetricSnapshot
	err         error
	calls       int
	maxGroups   int
	at          time.Time
	hasDeadline bool
}

func (r *recordingReliabilityMetricSnapshotStore) LatestReliabilityMetricSnapshots(ctx context.Context, at time.Time, maxGroups int) ([]persistence.ReliabilityMetricSnapshot, error) {
	r.calls++
	r.maxGroups = maxGroups
	r.at = at
	_, r.hasDeadline = ctx.Deadline()
	return r.snapshots, r.err
}

func TestSiteReliabilityAdapterUsesSingleBoundedDeadlineSnapshot(t *testing.T) {
	store := &recordingReliabilityMetricSnapshotStore{snapshots: []persistence.ReliabilityMetricSnapshot{
		{GroupID: "group-a", GroupName: "Primary", Score: 97},
		{GroupID: "group-b", GroupName: "DR", Score: 88},
	}}
	adapter := &siteReliabilityAdapter{snapshotStore: store}
	got := adapter.AllSiteReliabilityMetrics()
	if len(got) != 2 || got[0].Score != 97 || got[0].Labels["site_id"] != "group-a" || got[1].Labels["site_name"] != "DR" {
		t.Fatalf("site reliability metrics = %+v", got)
	}
	if store.calls != 1 || store.maxGroups != telemetry.MaxSiteReliabilityMetricSeries || !store.hasDeadline || store.at.IsZero() {
		t.Fatalf("snapshot call was not single/deadline/bounded: calls=%d max=%d deadline=%v at=%v", store.calls, store.maxGroups, store.hasDeadline, store.at)
	}
}

func TestSiteReliabilityAdapterFailsClosedOnSnapshotError(t *testing.T) {
	store := &recordingReliabilityMetricSnapshotStore{err: errors.New("forced reliability snapshot failure")}
	adapter := &siteReliabilityAdapter{snapshotStore: store}
	if got := adapter.AllSiteReliabilityMetrics(); got != nil {
		t.Fatalf("failed reliability snapshot returned partial metrics: %+v", got)
	}
	if store.calls != 1 || !store.hasDeadline {
		t.Fatalf("failed snapshot was not invoked once with deadline: calls=%d deadline=%v", store.calls, store.hasDeadline)
	}
}

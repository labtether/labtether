package main

import (
	"context"
	"errors"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/telemetry"
	"testing"
	"time"
)

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

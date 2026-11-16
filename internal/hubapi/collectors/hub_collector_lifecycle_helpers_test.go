package collectors_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/assets"
	collectorspkg "github.com/labtether/labtether/internal/hubapi/collectors"
	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/hubapi/testutil/hubdeps"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/persistence"
)

type lifecycleHelperAssetStoreWithErrors struct {
	persistence.AssetStore
	failUpsert map[string]error
}

func (s *lifecycleHelperAssetStoreWithErrors) UpsertAssetHeartbeat(req assets.HeartbeatRequest) (assets.Asset, error) {
	if err, ok := s.failUpsert[strings.TrimSpace(req.AssetID)]; ok {
		return assets.Asset{}, err
	}
	return s.AssetStore.UpsertAssetHeartbeat(req)
}

func TestCollectorLifecycleFailUpdatesStatusAndLogs(t *testing.T) {
	sut := hubdeps.New(t)
	store := testutil.NewRecordingHubCollectorStore()
	sut.HubCollectorStore = store

	collector := hubcollector.Collector{
		ID:            "collector-lifecycle-fail",
		AssetID:       "truenas-cluster-lifecycle-fail",
		CollectorType: hubcollector.CollectorTypeTrueNAS,
		Enabled:       true,
	}
	store.StatusByID[collector.ID] = collector

	lifecycle := collectorspkg.NewCollectorLifecycle(sut.CollectorsDeps(), collector, "truenas", hubcollector.CollectorTypeTrueNAS)
	lifecycle.Fail("missing base_url in config")

	updated, ok, err := store.GetHubCollector(collector.ID)
	if err != nil || !ok {
		t.Fatalf("collector status lookup failed: ok=%v err=%v", ok, err)
	}
	if updated.LastStatus != "error" {
		t.Fatalf("LastStatus = %q, want error", updated.LastStatus)
	}
	if updated.LastError != "missing base_url in config" {
		t.Fatalf("LastError = %q, want %q", updated.LastError, "missing base_url in config")
	}

	events, err := sut.LogStore.QueryEvents(logs.QueryRequest{
		Source: "truenas",
		From:   time.Unix(0, 0).UTC(),
		To:     time.Now().UTC().Add(24 * time.Hour),
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("QueryEvents() error = %v", err)
	}
	found := false
	for _, event := range events {
		if event.Message != "collector run failed: missing base_url in config" {
			continue
		}
		if event.Level != "error" {
			t.Fatalf("event.Level = %q, want error", event.Level)
		}
		if event.Fields["collector_id"] != collector.ID {
			t.Fatalf("collector_id field = %q, want %q", event.Fields["collector_id"], collector.ID)
		}
		if event.Fields["collector_type"] != hubcollector.CollectorTypeTrueNAS {
			t.Fatalf("collector_type field = %q, want %q", event.Fields["collector_type"], hubcollector.CollectorTypeTrueNAS)
		}
		found = true
		break
	}
	if !found {
		t.Fatalf("expected collector failure log event")
	}
}

func TestCollectorLifecyclePartialUpdatesStatusAndLogs(t *testing.T) {
	sut := hubdeps.New(t)
	store := testutil.NewRecordingHubCollectorStore()
	sut.HubCollectorStore = store

	collector := hubcollector.Collector{
		ID:            "collector-lifecycle-partial",
		AssetID:       "pbs-root-lifecycle-partial",
		CollectorType: hubcollector.CollectorTypePBS,
		Enabled:       true,
	}
	store.StatusByID[collector.ID] = collector

	lifecycle := collectorspkg.NewCollectorLifecycle(sut.CollectorsDeps(), collector, "pbs", hubcollector.CollectorTypePBS)
	lifecycle.Partial("no datastores discovered from PBS")

	updated, ok, err := store.GetHubCollector(collector.ID)
	if err != nil || !ok {
		t.Fatalf("collector status lookup failed: ok=%v err=%v", ok, err)
	}
	if updated.LastStatus != "partial" {
		t.Fatalf("LastStatus = %q, want partial", updated.LastStatus)
	}
	if updated.LastError != "no datastores discovered from PBS" {
		t.Fatalf("LastError = %q, want %q", updated.LastError, "no datastores discovered from PBS")
	}

	events, err := sut.LogStore.QueryEvents(logs.QueryRequest{
		Source: "pbs",
		From:   time.Unix(0, 0).UTC(),
		To:     time.Now().UTC().Add(24 * time.Hour),
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("QueryEvents() error = %v", err)
	}
	found := false
	for _, event := range events {
		if event.Message != "collector run partial: no datastores discovered from PBS" {
			continue
		}
		if event.Level != "warn" {
			t.Fatalf("event.Level = %q, want warn", event.Level)
		}
		if event.Fields["collector_id"] != collector.ID {
			t.Fatalf("collector_id field = %q, want %q", event.Fields["collector_id"], collector.ID)
		}
		found = true
		break
	}
	if !found {
		t.Fatalf("expected collector partial log event")
	}
}

func TestRefreshCollectorParentAssetReturnsFalseOnHeartbeatFailure(t *testing.T) {
	sut := hubdeps.New(t)
	sut.AssetStore = &lifecycleHelperAssetStoreWithErrors{
		AssetStore: sut.AssetStore,
		failUpsert: map[string]error{
			"cluster-refresh-fail": errors.New("forced upsert error"),
		},
	}

	collector := hubcollector.Collector{
		ID:            "collector-refresh-fail",
		AssetID:       "cluster-refresh-fail",
		CollectorType: hubcollector.CollectorTypeDocker,
		Enabled:       true,
	}

	if _, ok := sut.CollectorsDeps().KeepConnectorClusterAssetAlive(collector, "docker", 0, "hub collector docker"); ok {
		t.Fatalf("expected keepConnectorClusterAssetAlive to fail")
	}
}

func TestConnectorSnapshotAssetFromHeartbeatClonesMetadata(t *testing.T) {
	req := assets.HeartbeatRequest{
		AssetID: "asset-1",
		Type:    "container",
		Name:    "nginx",
		Source:  "docker",
		Metadata: map[string]string{
			"state": "running",
		},
	}

	snapshot := collectorspkg.ConnectorSnapshotAssetFromHeartbeat(req, "docker-container")
	req.Metadata["state"] = "stopped"

	if snapshot.ID != "asset-1" {
		t.Fatalf("snapshot ID = %q, want asset-1", snapshot.ID)
	}
	if snapshot.Kind != "docker-container" {
		t.Fatalf("snapshot Kind = %q, want docker-container", snapshot.Kind)
	}
	if snapshot.Metadata["state"] != "running" {
		t.Fatalf("snapshot metadata state = %q, want running", snapshot.Metadata["state"])
	}
}

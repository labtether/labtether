package remotewrite

import (
	"testing"
	"time"

	"github.com/labtether/labtether/internal/telemetry"
)

func TestBuildBatchUsesPrometheusNamesAndLabelsAndAdvancesBothCursors(t *testing.T) {
	now := time.Now().UTC()
	batch, err := BuildBatch(Cursor{AssetSampleID: 10, HubSampleID: 20}, []AssetSampleRow{{
		ID: 11, AssetID: "asset-1", AssetName: "server", AssetType: "docker-container", Platform: "linux",
		DockerHost: "host-1", DockerImage: "image:1", DockerStack: "stack",
		Metric: "disk.used-bytes", Unit: "bytes", Value: 42, CollectedAt: now,
		Labels: map[string]string{"mount_point": "/data", "unsupported": "must-not-project"},
	}}, []HubSampleRow{{
		ID: 21, Scope: telemetry.MetricScopeHubAlerts, Metric: telemetry.MetricAlertEvaluationDurationMs,
		Unit: "ms", Value: 9, CollectedAt: now.Add(time.Millisecond),
		Labels: map[string]string{"rule_id": "rule-1", "rule_name": "CPU"},
	}})
	if err != nil {
		t.Fatalf("BuildBatch: %v", err)
	}
	if batch.Next != (Cursor{AssetSampleID: 11, HubSampleID: 21}) || len(batch.Samples) != 2 {
		t.Fatalf("batch mismatch: %+v", batch)
	}
	byName := make(map[string]SampleWithLabels)
	for _, sample := range batch.Samples {
		byName[sample.Labels["__name__"]] = sample
	}
	asset := byName["labtether_disk_used_bytes"]
	if asset.Labels["asset_id"] != "asset-1" || asset.Labels["mount_point"] != "/data" || asset.Labels["unsupported"] != "" {
		t.Fatalf("asset labels mismatch: %#v", asset.Labels)
	}
	for _, required := range []string{"asset_name", "asset_type", "group", "platform", "docker_host", "docker_image", "docker_stack", "interface", "process_name"} {
		if _, ok := asset.Labels[required]; !ok {
			t.Fatalf("missing fixed asset label %q: %#v", required, asset.Labels)
		}
	}
	hub := byName["labtether_alert_evaluation_duration_ms"]
	if hub.Labels["scope"] != telemetry.MetricScopeHubAlerts || hub.Labels["rule_id"] != "rule-1" || hub.Labels["rule_name"] != "CPU" {
		t.Fatalf("hub labels mismatch: %#v", hub.Labels)
	}
}

func TestBuildBatchCanonicalizesProjectedCollisionWithoutCursorGap(t *testing.T) {
	now := time.Now().UTC()
	rows := []AssetSampleRow{
		{ID: 1, AssetID: "asset", AssetName: "server", AssetType: "linux", Platform: "linux", Metric: "cpu-used", Unit: "percent", Value: 1, CollectedAt: now},
		{ID: 2, AssetID: "asset", AssetName: "server", AssetType: "linux", Platform: "linux", Metric: "cpu_used", Unit: "percent", Value: 2, CollectedAt: now},
	}
	batch, err := BuildBatch(Cursor{}, rows, nil)
	if err != nil {
		t.Fatalf("BuildBatch: %v", err)
	}
	if batch.Next.AssetSampleID != 2 || len(batch.Samples) != 1 || batch.Samples[0].Value != 2 {
		t.Fatalf("canonical collision result = %+v", batch)
	}
}

func TestBuildBatchFailsClosedOnMalformedPersistedRow(t *testing.T) {
	_, err := BuildBatch(Cursor{}, []AssetSampleRow{{
		ID: 1, AssetID: "asset", AssetName: "server", AssetType: "linux", Platform: "linux",
		Metric: "cpu", Unit: "percent", Value: 1, CollectedAt: time.Now(),
		Labels: map[string]string{"bad\x00label": "value"},
	}}, nil)
	if err == nil {
		t.Fatal("expected malformed persisted row to fail closed")
	}
}

func TestBuildBatchMatchesReservedAndDockerScrapeProjection(t *testing.T) {
	now := time.Now().UTC()
	batch, err := BuildBatch(Cursor{}, []AssetSampleRow{
		{
			ID: 1, AssetID: "linux", AssetName: "server", AssetType: "linux", Platform: "linux",
			DockerHost: "must-not-project", DockerImage: "must-not-project", DockerStack: "must-not-project",
			Metric: "cpu_used_percent", Unit: "percent", Value: 1, CollectedAt: now,
		},
		{
			ID: 2, AssetID: "linux", AssetName: "server", AssetType: "linux", Platform: "linux",
			Metric: telemetry.MetricAlertsFiring, Unit: "count", Value: 99, CollectedAt: now,
		},
	}, nil)
	if err != nil {
		t.Fatalf("BuildBatch: %v", err)
	}
	if batch.Next.AssetSampleID != 2 || len(batch.Samples) != 1 {
		t.Fatalf("reserved projection result = %+v", batch)
	}
	labels := batch.Samples[0].Labels
	if labels["docker_host"] != "" || labels["docker_image"] != "" || labels["docker_stack"] != "" {
		t.Fatalf("non-container projected docker metadata: %#v", labels)
	}
}

func TestBuildBatchStrictOrderingAndNewestIdenticalTimestampWins(t *testing.T) {
	now := time.Now().UTC()
	base := AssetSampleRow{
		AssetID: "asset", AssetName: "server", AssetType: "linux", Platform: "linux",
		Metric: "cpu_used_percent", Unit: "percent", CollectedAt: now,
	}
	first, second := base, base
	first.ID, first.Value = 1, 1
	second.ID, second.Value = 2, 2
	batch, err := BuildBatch(Cursor{}, []AssetSampleRow{first, second}, nil)
	if err != nil {
		t.Fatalf("BuildBatch: %v", err)
	}
	if batch.Next.AssetSampleID != 2 || len(batch.Samples) != 1 || batch.Samples[0].Value != 2 {
		t.Fatalf("identical timestamp result = %+v", batch)
	}
	if _, err := BuildBatch(Cursor{}, []AssetSampleRow{second, first}, nil); err == nil {
		t.Fatal("out-of-order persistence page unexpectedly succeeded")
	}
}

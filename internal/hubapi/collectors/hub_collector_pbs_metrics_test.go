package collectors

import (
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/telemetry"
)

func TestPBSDatastoreMetricSamplesUseExistingCollectorSnapshotTruthfully(t *testing.T) {
	collectedAt := time.Unix(1_700_000_000, 0).UTC()
	samples := PBSDatastoreMetricSamples(PBSDatastoreMetricSnapshot{
		AssetID: "pbs-datastore-backup--collector-1", Datastore: "backup", CollectedAt: collectedAt,
		Total: 1000, Used: 400, Available: 600,
		BackupCountKnown: true, BackupCount: 7,
		LatestBackupEpoch: collectedAt.Add(-2 * time.Hour).Unix(),
		GCPendingKnown:    true, GCPendingBytes: 123,
	})
	if len(samples) != 6 {
		t.Fatalf("PBS metric samples = %d, want 6: %+v", len(samples), samples)
	}
	byMetric := make(map[string]telemetry.MetricSample, len(samples))
	for _, sample := range samples {
		byMetric[sample.Metric] = sample
		if sample.AssetID != "pbs-datastore-backup--collector-1" || sample.CollectedAt != collectedAt || sample.Labels["datastore"] != "backup" {
			t.Fatalf("PBS sample identity/timestamp mismatch: %+v", sample)
		}
		if _, err := telemetry.MetricSampleEnvelopeBytes(sample); err != nil {
			t.Fatalf("PBS sample envelope invalid: %v", err)
		}
	}
	wants := map[string]float64{
		telemetry.MetricStorageTotalBytes:     1000,
		telemetry.MetricStorageUsedBytes:      400,
		telemetry.MetricStorageAvailableBytes: 600,
		telemetry.MetricBackupCount:           7,
		telemetry.MetricBackupAgeSeconds:      7200,
		telemetry.MetricGCPendingBytes:        123,
	}
	for metric, want := range wants {
		if got := byMetric[metric].Value; got != want {
			t.Errorf("PBS %s = %v, want %v", metric, got, want)
		}
	}
}

func TestPBSDatastoreMetricSamplesDoNotFabricateUnavailableOptionalData(t *testing.T) {
	invalidStorage := PBSDatastoreMetricSamples(PBSDatastoreMetricSnapshot{
		AssetID: "pbs-datastore-backup--collector-1", Datastore: "backup", CollectedAt: time.Now().UTC(),
		Total: 100, Used: 60, Available: 60,
	})
	if len(invalidStorage) != 0 {
		t.Fatalf("inconsistent/unknown PBS values produced metrics: %+v", invalidStorage)
	}
	overlongIdentity := PBSDatastoreMetricSamples(PBSDatastoreMetricSnapshot{
		AssetID: "pbs-datastore-backup--collector-1", Datastore: strings.Repeat("x", telemetry.MaxMetricLabelValueBytes+1),
		CollectedAt: time.Now().UTC(), Total: 100, Used: 40, Available: 60,
	})
	if len(overlongIdentity) != 0 {
		t.Fatalf("overlong PBS identity produced metrics: %+v", overlongIdentity)
	}
	futureBackup := PBSDatastoreMetricSamples(PBSDatastoreMetricSnapshot{
		AssetID: "pbs-datastore-backup--collector-1", Datastore: "backup", CollectedAt: time.Now().UTC(),
		LatestBackupEpoch: time.Now().UTC().Add(time.Hour).Unix(),
	})
	if len(futureBackup) != 0 {
		t.Fatalf("future PBS backup timestamp produced a fabricated age: %+v", futureBackup)
	}
}

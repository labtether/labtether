package collectors

import (
	"github.com/labtether/labtether/internal/connectors/pbs"
	"github.com/labtether/labtether/internal/telemetry"
	"strings"
	"time"
)

// PBSDatastoreMetricSnapshot is the exact data already loaded during one PBS
// collector pass. It avoids an export-only second poll while preserving which
// optional API calls actually succeeded.
type PBSDatastoreMetricSnapshot struct {
	AssetID           string
	Datastore         string
	CollectedAt       time.Time
	Total             int64
	Used              int64
	Available         int64
	BackupCountKnown  bool
	BackupCount       int
	LatestBackupEpoch int64
	GCPendingKnown    bool
	GCPendingBytes    int64
}

func PBSDatastoreMetricSamples(snapshot PBSDatastoreMetricSnapshot) []telemetry.MetricSample {
	if strings.TrimSpace(snapshot.AssetID) == "" || strings.TrimSpace(snapshot.Datastore) == "" || snapshot.CollectedAt.IsZero() {
		return nil
	}
	labels := map[string]string{"datastore": strings.TrimSpace(snapshot.Datastore)}
	out := make([]telemetry.MetricSample, 0, 6)
	appendSample := func(metric, unit string, value float64) {
		sample := telemetry.MetricSample{
			AssetID: snapshot.AssetID, Metric: metric, Unit: unit, Value: value,
			CollectedAt: snapshot.CollectedAt.UTC(), Labels: labels,
		}
		if _, err := telemetry.MetricSampleEnvelopeBytes(sample); err == nil {
			out = append(out, sample)
		}
	}
	if snapshot.Total > 0 && snapshot.Used >= 0 && snapshot.Used <= snapshot.Total &&
		snapshot.Available >= 0 && snapshot.Available <= snapshot.Total && snapshot.Used <= snapshot.Total-snapshot.Available {
		appendSample(telemetry.MetricStorageTotalBytes, "bytes", float64(snapshot.Total))
		appendSample(telemetry.MetricStorageUsedBytes, "bytes", float64(snapshot.Used))
		appendSample(telemetry.MetricStorageAvailableBytes, "bytes", float64(snapshot.Available))
	}
	if snapshot.BackupCountKnown && snapshot.BackupCount >= 0 {
		appendSample(telemetry.MetricBackupCount, "count", float64(snapshot.BackupCount))
	}
	if snapshot.LatestBackupEpoch > 0 {
		age := snapshot.CollectedAt.Sub(time.Unix(snapshot.LatestBackupEpoch, 0)).Seconds()
		if age >= 0 {
			appendSample(telemetry.MetricBackupAgeSeconds, "seconds", age)
		}
	}
	if snapshot.GCPendingKnown && snapshot.GCPendingBytes >= 0 {
		appendSample(telemetry.MetricGCPendingBytes, "bytes", float64(snapshot.GCPendingBytes))
	}
	return out
}

func latestPBSSnapshotEpoch(snapshots []pbs.BackupSnapshot) int64 {
	var latest int64
	for _, snapshot := range snapshots {
		if snapshot.BackupTime > latest {
			latest = snapshot.BackupTime
		}
	}
	return latest
}

func firstNonZeroInt64(values ...int64) int64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func normalizePBSDatastoreStatus(mountStatus, maintenance string) string {
	normalizedMount := strings.ToLower(strings.TrimSpace(mountStatus))
	normalizedMaintenance := strings.ToLower(strings.TrimSpace(maintenance))

	if normalizedMaintenance == "offline" || normalizedMaintenance == "delete" || normalizedMaintenance == "unmount" {
		return "offline"
	}
	if normalizedMount == "notmounted" {
		return "offline"
	}
	if normalizedMaintenance == "read-only" {
		return "stale"
	}
	return "online"
}

func FirstNonEmptyString(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

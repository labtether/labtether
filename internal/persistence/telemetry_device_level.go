package persistence

import (
	"strings"
	"time"

	"github.com/labtether/labtether/internal/telemetry"
)

// A device's per-disk, per-interface and per-process samples share metric
// names with its own samples (disk_used_percent from the heartbeat and from
// every mount). Device-level answers (latest snapshot, charts) mixed them: a
// Windows machine with C: and M: reported M:'s free space against C:'s total,
// and its disk chart jumped between the two. These label keys mark samples
// that belong to a part of the device, not the device.
var deviceSubEntityLabelKeys = []string{"mount_point", "interface", "process_pid"}

// deviceLevelSampleFilter is the SQL form of isDeviceLevelSample.
const deviceLevelSampleFilter = `(labels IS NULL OR NOT (labels ?| array['mount_point','interface','process_pid']))`

// recentMountSampleWindow bounds the fallback that fills disk metrics only
// the per-mount samples carry (disk_total_bytes and friends).
const (
	recentMountSampleWindow = `interval '10 minutes'`
	recentMountSampleAge    = 10 * time.Minute
)

// primaryMountOrderSQL orders per-mount samples so the system disk comes
// first: "/" on Unix, then C: on Windows, then by name. All of a device's
// disk totals then come from the same mount.
const primaryMountOrderSQL = `CASE WHEN labels->>'mount_point' = '/' THEN 0 WHEN labels->>'mount_point' ~* '^c:[\\/]?$' THEN 1 ELSE 2 END, labels->>'mount_point'`

func isDeviceLevelSample(labels map[string]string) bool {
	for _, key := range deviceSubEntityLabelKeys {
		if _, ok := labels[key]; ok {
			return false
		}
	}
	return true
}

func primaryMountRank(mountPoint string) int {
	switch lower := strings.ToLower(strings.TrimSpace(mountPoint)); {
	case lower == "/":
		return 0
	case lower == "c:" || lower == `c:\` || lower == "c:/":
		return 1
	}
	return 2
}

// betterMountSample reports whether candidate should replace current when
// filling a device metric from per-mount samples: newer collection first,
// then the system disk.
func betterMountSample(candidate, current telemetry.MetricSample) bool {
	if !candidate.CollectedAt.Equal(current.CollectedAt) {
		return candidate.CollectedAt.After(current.CollectedAt)
	}
	cr, kr := primaryMountRank(candidate.Labels["mount_point"]), primaryMountRank(current.Labels["mount_point"])
	if cr != kr {
		return cr < kr
	}
	return candidate.Labels["mount_point"] < current.Labels["mount_point"]
}

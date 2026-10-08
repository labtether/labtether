package main

import (
	"fmt"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/connectors/proxmox"
	proxmoxpkg "github.com/labtether/labtether/internal/hubapi/proxmox"
	"github.com/labtether/labtether/internal/telemetry"
	"math"
	"strings"
	"testing"
	"time"
)

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

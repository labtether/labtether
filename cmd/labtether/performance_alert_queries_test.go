package main

import (
	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/incidents"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/telemetry"
	"testing"
	"time"
)

func TestMetricThresholdAlertUsesBatchMetricSeriesQuery(t *testing.T) {
	sut := newTestAPIServer(t)

	telemetryCounter := &countingTelemetryStore{TelemetryStore: sut.telemetryStore}
	sut.telemetryStore = telemetryCounter

	seedAssetViaHeartbeat(t, sut, "batch-threshold-asset-1", "Batch Threshold One")
	seedAssetViaHeartbeat(t, sut, "batch-threshold-asset-2", "Batch Threshold Two")
	seedMetricSamples(t, sut, "batch-threshold-asset-1", telemetry.MetricCPUUsedPercent, 55)
	seedMetricSamples(t, sut, "batch-threshold-asset-2", telemetry.MetricCPUUsedPercent, 97)

	rule := alerts.Rule{
		ID:          "rule-batch-threshold",
		Name:        "Batch Threshold",
		Kind:        alerts.RuleKindMetricThreshold,
		Severity:    alerts.SeverityCritical,
		TargetScope: alerts.TargetScopeAsset,
		Condition: map[string]any{
			"metric":    telemetry.MetricCPUUsedPercent,
			"operator":  ">",
			"threshold": float64(90),
		},
		Targets: []alerts.RuleTarget{
			{AssetID: "batch-threshold-asset-1"},
			{AssetID: "batch-threshold-asset-2"},
		},
	}

	triggered, err := sut.evaluateMetricThresholdWithPrefetch(rule, nil, nil)
	if err != nil {
		t.Fatalf("evaluateMetricThresholdWithPrefetch() error = %v", err)
	}
	if !triggered {
		t.Fatalf("expected threshold alert to trigger")
	}
	if telemetryCounter.metricSeriesBatchCalls != 1 {
		t.Fatalf("expected one batch metric query, got %d", telemetryCounter.metricSeriesBatchCalls)
	}
	if telemetryCounter.seriesMetricCalls != 0 {
		t.Fatalf("expected per-asset metric queries to be skipped, got %d", telemetryCounter.seriesMetricCalls)
	}
	if telemetryCounter.seriesCalls != 0 {
		t.Fatalf("expected full series queries to be skipped, got %d", telemetryCounter.seriesCalls)
	}
}

func TestMetricDeadmanAlertUsesBatchActivityQuery(t *testing.T) {
	sut := newTestAPIServer(t)

	telemetryCounter := &countingTelemetryStore{TelemetryStore: sut.telemetryStore}
	sut.telemetryStore = telemetryCounter

	seedAssetViaHeartbeat(t, sut, "batch-deadman-asset-1", "Batch Deadman One")
	seedAssetViaHeartbeat(t, sut, "batch-deadman-asset-2", "Batch Deadman Two")
	seedMetricSamples(t, sut, "batch-deadman-asset-1", telemetry.MetricCPUUsedPercent, 40)

	rule := alerts.Rule{
		ID:          "rule-batch-deadman",
		Name:        "Batch Deadman",
		Kind:        alerts.RuleKindMetricDeadman,
		Severity:    alerts.SeverityHigh,
		TargetScope: alerts.TargetScopeAsset,
		Targets: []alerts.RuleTarget{
			{AssetID: "batch-deadman-asset-1"},
			{AssetID: "batch-deadman-asset-2"},
		},
	}

	triggered, err := sut.evaluateMetricDeadmanWithPrefetch(rule, nil, nil)
	if err != nil {
		t.Fatalf("evaluateMetricDeadmanWithPrefetch() error = %v", err)
	}
	if !triggered {
		t.Fatalf("expected deadman alert to trigger for missing asset telemetry")
	}
	if telemetryCounter.hasSamplesCalls != 1 {
		t.Fatalf("expected one batch activity query, got %d", telemetryCounter.hasSamplesCalls)
	}
	if telemetryCounter.seriesCalls != 0 {
		t.Fatalf("expected full series deadman queries to be skipped, got %d", telemetryCounter.seriesCalls)
	}
}

func TestLogPatternAlertUsesSingleTargetedLogQuery(t *testing.T) {
	sut := newTestAPIServer(t)

	logCounter := &countingLogStore{LogStore: sut.logStore}
	sut.logStore = logCounter

	seedAssetViaHeartbeat(t, sut, "batch-log-asset-1", "Batch Log One")
	seedAssetViaHeartbeat(t, sut, "batch-log-asset-2", "Batch Log Two")
	now := time.Now().UTC()
	if err := sut.logStore.AppendEvent(logs.Event{
		ID:        "batch-log-event-1",
		AssetID:   "batch-log-asset-2",
		Source:    "agent",
		Level:     "error",
		Message:   "kernel panic happened",
		Timestamp: now,
	}); err != nil {
		t.Fatalf("append event failed: %v", err)
	}

	rule := alerts.Rule{
		ID:          "rule-batch-log",
		Name:        "Batch Log Pattern",
		Kind:        alerts.RuleKindLogPattern,
		Severity:    alerts.SeverityHigh,
		TargetScope: alerts.TargetScopeAsset,
		Condition: map[string]any{
			"pattern": "panic",
			"level":   "error",
		},
		Targets: []alerts.RuleTarget{
			{AssetID: "batch-log-asset-1"},
			{AssetID: "batch-log-asset-2"},
		},
	}

	triggered, err := sut.evaluateLogPatternWithPrefetch(rule, nil, nil)
	if err != nil {
		t.Fatalf("evaluateLogPatternWithPrefetch() error = %v", err)
	}
	if !triggered {
		t.Fatalf("expected log pattern alert to trigger")
	}
	if logCounter.queryEventsCalls != 1 {
		t.Fatalf("expected one targeted log query, got %d", logCounter.queryEventsCalls)
	}
	if len(logCounter.lastQueryEventsReq.GroupAssetIDs) != 2 {
		t.Fatalf("expected targeted asset filter to include both assets, got %#v", logCounter.lastQueryEventsReq.GroupAssetIDs)
	}
}

func TestMaybeAutoCreateIncidentUsesDirectExistenceCheck(t *testing.T) {
	sut := newTestAPIServer(t)

	incidentCounter := &countingIncidentStore{IncidentStore: sut.incidentStore}
	sut.incidentStore = incidentCounter

	existing, err := sut.incidentStore.CreateIncident(incidents.CreateIncidentRequest{
		Title:    "Existing Auto Incident",
		Severity: incidents.SeverityCritical,
		Source:   incidents.SourceAlertAuto,
	})
	if err != nil {
		t.Fatalf("create auto incident: %v", err)
	}
	if _, err := sut.incidentStore.LinkIncidentAlert(existing.ID, incidents.LinkAlertRequest{
		AlertInstanceID: "inst-dup",
		LinkType:        incidents.LinkTypeTrigger,
	}); err != nil {
		t.Fatalf("link existing auto incident: %v", err)
	}

	sut.maybeAutoCreateIncident(alerts.Rule{
		ID:       "rule-dup",
		Name:     "Duplicate CPU Saturation",
		Severity: alerts.SeverityCritical,
	}, alerts.AlertInstance{
		ID:        "inst-dup",
		StartedAt: time.Now().Add(-10 * time.Minute),
	})

	autoIncidents, err := sut.incidentStore.ListIncidents(persistence.IncidentFilter{
		Limit:  10,
		Source: incidents.SourceAlertAuto,
	})
	if err != nil {
		t.Fatalf("list auto incidents: %v", err)
	}
	if len(autoIncidents) != 1 {
		t.Fatalf("expected duplicate auto incident creation to be skipped, incidents=%d", len(autoIncidents))
	}
	if incidentCounter.hasAutoIncidentCalls != 1 {
		t.Fatalf("expected direct existence helper to be used once, got %d", incidentCounter.hasAutoIncidentCalls)
	}
	if incidentCounter.listIncidentAlertLinksCalls != 0 {
		t.Fatalf("expected no per-incident link scans, got %d", incidentCounter.listIncidentAlertLinksCalls)
	}
}

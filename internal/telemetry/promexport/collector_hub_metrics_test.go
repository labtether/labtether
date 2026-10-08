package promexport

import (
	"github.com/labtether/labtether/internal/telemetry"
	"github.com/prometheus/client_golang/prometheus"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCollectorHubMetricsKeepPublicNamesAndCannotCollideWithAsset(t *testing.T) {
	src := &mockSource{
		snapshots: map[string][]LabeledMetric{
			"labtether-hub": {
				{Metric: telemetry.MetricCPUUsedPercent, Value: 17},
				// Sanitizes to a reserved public hub name and must be dropped.
				{Metric: "alerts-firing", Value: 999},
				// Sanitizes to the reserved static asset_info family.
				{Metric: "asset_info", Value: 999},
				{Metric: "asset-info", Value: 999},
				{Metric: "asset.info", Value: 999},
				{Metric: "foo-bar", Value: 1},
				{Metric: "foo.bar", Value: 2},
				{Metric: "foo_bar", Value: 3},
				{Metric: "hidden_metric", Value: 4, CollectedAt: time.Unix(1, 0), Labels: map[string]string{"evil": "a"}},
				{Metric: "hidden_metric", Value: 5, CollectedAt: time.Unix(2, 0), Labels: map[string]string{"evil": "b"}},
			},
			"other-asset": {
				{Metric: "foo.bar", Value: 9},
			},
		},
		metas: map[string]AssetMeta{
			"labtether-hub": {Name: "Real Enrolled Asset", Type: "linux"},
			"other-asset":   {Name: "Other Asset", Type: "linux"},
		},
		hub: map[string][]LabeledMetric{
			telemetry.MetricScopeHubAlerts: {
				{Metric: telemetry.MetricAlertsFiring, Value: 2},
				{Metric: telemetry.MetricAlertsRules, Value: 7},
				{Metric: telemetry.MetricAlertEvaluationDurationMs, Value: 8, Labels: map[string]string{"rule_id": "rule-a", "rule_name": "duplicate"}},
				{Metric: telemetry.MetricAlertEvaluationDurationMs, Value: 9, Labels: map[string]string{"rule_id": "rule-b", "rule_name": "duplicate"}},
				// Invalid schemas are ignored defensively.
				{Metric: telemetry.MetricAlertEvaluationDurationMs, Value: 99, Labels: map[string]string{"rule_name": "missing-id"}},
				{Metric: telemetry.MetricCPUUsedPercent, Value: 99},
			},
			telemetry.MetricScopeHubReliability: {
				{Metric: telemetry.MetricSiteReliabilityScore, Value: 98, Labels: map[string]string{"site_id": "site-1", "site_name": "Primary"}},
			},
		},
	}

	metrics := collectAll(NewCollector(src))
	if len(metrics) != 11 { // 2 asset_info + CPU + 2 foo + hidden dedupe + 5 hub
		t.Fatalf("metric count = %d, want 11", len(metrics))
	}

	publicHubNames := map[string]int{
		"labtether_" + telemetry.MetricAlertsFiring:              1,
		"labtether_" + telemetry.MetricAlertsRules:               1,
		"labtether_" + telemetry.MetricAlertEvaluationDurationMs: 2,
		"labtether_" + telemetry.MetricSiteReliabilityScore:      1,
	}
	seenHubNames := make(map[string]int)
	evaluationIDs := make(map[string]float64)
	assetInfoCount := 0
	aliasValues := make(map[string]float64)
	hiddenLabelCount := 0
	for _, metric := range metrics {
		desc := metric.Desc().String()
		if strings.Contains(desc, "labtether_hub_") {
			t.Fatalf("hub metric public name changed: %s", desc)
		}
		pb := metricToDTO(metric)
		if hasMetricName(metric, "labtether_asset_info") {
			assetInfoCount++
			if labelValue(pb, "asset_id") == "labtether-hub" && labelValue(pb, "asset_name") != "Real Enrolled Asset" {
				t.Fatalf("colliding real asset metadata changed: %+v", pb.GetLabel())
			}
			continue
		}
		if hasMetricName(metric, "labtether_"+telemetry.MetricCPUUsedPercent) {
			if labelValue(pb, "asset_id") != "labtether-hub" || hasLabel(pb, "scope") {
				t.Fatalf("asset CPU identity/labels changed: %+v", pb.GetLabel())
			}
			continue
		}
		if hasMetricName(metric, "labtether_foo_bar") {
			aliasValues[labelValue(pb, "asset_id")] = pb.GetGauge().GetValue()
			continue
		}
		if hasMetricName(metric, "labtether_hidden_metric") {
			hiddenLabelCount++
			if got := pb.GetGauge().GetValue(); got != 5 {
				t.Fatalf("hidden-label collision selected value %v, want newest value 5", got)
			}
			continue
		}
		for name := range publicHubNames {
			if hasMetricName(metric, name) {
				seenHubNames[name]++
				if labelValue(pb, "scope") == "" || hasLabel(pb, "asset_id") {
					t.Fatalf("hub metric missing scope or gained asset identity: %+v", pb.GetLabel())
				}
				if name == "labtether_"+telemetry.MetricAlertEvaluationDurationMs {
					if got := labelNames(pb); !reflect.DeepEqual(got, []string{"rule_id", "rule_name", "scope"}) {
						t.Fatalf("evaluation labels = %v", got)
					}
					evaluationIDs[labelValue(pb, "rule_id")] = pb.GetGauge().GetValue()
				}
			}
		}
	}
	if assetInfoCount != 2 {
		t.Fatalf("asset_info count = %d, want 2", assetInfoCount)
	}
	if !reflect.DeepEqual(aliasValues, map[string]float64{"labtether-hub": 3, "other-asset": 9}) {
		t.Fatalf("sanitized alias dedupe crossed asset boundary or chose wrong value: %v", aliasValues)
	}
	if hiddenLabelCount != 1 {
		t.Fatalf("unsupported-label exported identity count = %d, want 1", hiddenLabelCount)
	}
	if !reflect.DeepEqual(seenHubNames, publicHubNames) {
		t.Fatalf("public hub metric names/counts = %v, want %v", seenHubNames, publicHubNames)
	}
	if !reflect.DeepEqual(evaluationIDs, map[string]float64{"rule-a": 8, "rule-b": 9}) {
		t.Fatalf("duplicate-name rule series collapsed: %v", evaluationIDs)
	}

	registry := prometheus.NewRegistry()
	if err := registry.Register(NewCollector(src)); err != nil {
		t.Fatalf("register collector: %v", err)
	}
	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("real registry gather found descriptor/series conflict: %v", err)
	}
	familyCounts := make(map[string]int, len(families))
	for _, family := range families {
		familyCounts[family.GetName()] = len(family.GetMetric())
	}
	if familyCounts["labtether_asset_info"] != 2 || familyCounts["labtether_alert_evaluation_duration_ms"] != 2 {
		t.Fatalf("unexpected gathered family counts: %v", familyCounts)
	}
	if familyCounts["labtether_foo_bar"] != 2 {
		t.Fatalf("same-asset sanitized aliases were not deduplicated: %v", familyCounts)
	}
	if familyCounts["labtether_hidden_metric"] != 1 {
		t.Fatalf("hidden labels produced duplicate exported series: %v", familyCounts)
	}
	if _, exists := familyCounts["labtether_hub_alerts_firing"]; exists {
		t.Fatalf("legacy dashboard metric name changed: %v", familyCounts)
	}

	handler := NewHandler(src)
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("real HTTP scrape status = %d, body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for name := range publicHubNames {
		if !strings.Contains(body, name) {
			t.Fatalf("HTTP scrape missing public family %q", name)
		}
	}
	if strings.Contains(body, "labtether_hub_") {
		t.Fatalf("HTTP scrape changed public hub metric names: %s", body)
	}
}

func TestDynamicDescriptorsAreScrapeLocalAndAliasHelpIsStable(t *testing.T) {
	cache := make(map[string]*prometheus.Desc)
	first, ok := metricDesc(cache, "foo-bar")
	if !ok {
		t.Fatal("first descriptor rejected")
	}
	alias, ok := metricDesc(cache, "foo.bar")
	if !ok {
		t.Fatal("alias descriptor rejected")
	}
	if first != alias || len(cache) != 1 {
		t.Fatalf("sanitized aliases did not share one scrape-local descriptor: first=%p alias=%p cache=%d", first, alias, len(cache))
	}
	if got := first.String(); !strings.Contains(got, `fqName: "labtether_foo_bar"`) || !strings.Contains(got, `help: "LabTether metric: foo_bar"`) {
		t.Fatalf("descriptor did not use stable sanitized identity/help: %s", got)
	}

	for i := 0; i < 1000; i++ {
		rotating := make(map[string]*prometheus.Desc)
		if _, ok := metricDesc(rotating, "rotating.metric."+time.Unix(int64(i), 0).Format("150405.000000000")); !ok || len(rotating) != 1 {
			t.Fatalf("scrape-local descriptor cache iteration %d = %d entries", i, len(rotating))
		}
	}
}

func TestHubDescriptorCanonicalizesWhitespaceBeforeGlobalCache(t *testing.T) {
	canonical, ok := hubMetricDesc(telemetry.MetricScopeHubAlerts, telemetry.MetricAlertsFiring)
	if !ok {
		t.Fatal("canonical hub descriptor rejected")
	}
	for i := 1; i <= 256; i++ {
		scope := strings.Repeat(" ", i) + telemetry.MetricScopeHubAlerts + strings.Repeat(" ", i)
		metric := strings.Repeat(" ", i) + telemetry.MetricAlertsFiring + strings.Repeat(" ", i)
		desc, ok := hubMetricDesc(scope, metric)
		if !ok || desc != canonical {
			t.Fatalf("whitespace variant %d created a distinct/rejected descriptor: ok=%v canonical=%p got=%p", i, ok, canonical, desc)
		}
	}
	if got := canonical.String(); !strings.Contains(got, `fqName: "labtether_alerts_firing"`) {
		t.Fatalf("hub descriptor public name changed: %s", got)
	}

	src := &mockSource{hub: map[string][]LabeledMetric{
		" " + telemetry.MetricScopeHubAlerts + " ": {{Metric: " " + telemetry.MetricAlertsFiring + " ", Value: 3}},
	}}
	metrics := collectAll(NewCollector(src))
	if len(metrics) != 1 || !hasMetricName(metrics[0], "labtether_"+telemetry.MetricAlertsFiring) {
		t.Fatalf("whitespace hub sample did not emit canonical public family: %+v", metrics)
	}
}

func TestCollectorFailsClosedOnInvalidEnvelopesAndMetadata(t *testing.T) {
	cases := []struct {
		name string
		src  *mockSource
	}{
		{
			name: "oversized metric",
			src: &mockSource{
				snapshots: map[string][]LabeledMetric{"asset-1": {{Metric: strings.Repeat("m", telemetry.MaxMetricNameBytes+1), Value: 1}}},
				metas:     map[string]AssetMeta{"asset-1": {Name: "Asset"}},
			},
		},
		{
			name: "oversized label",
			src: &mockSource{
				snapshots: map[string][]LabeledMetric{"asset-1": {{Metric: "metric", Value: 1, Labels: map[string]string{"mount_point": strings.Repeat("v", telemetry.MaxMetricLabelValueBytes+1)}}}},
				metas:     map[string]AssetMeta{"asset-1": {Name: "Asset"}},
			},
		},
		{
			name: "NUL metadata",
			src: &mockSource{
				snapshots: map[string][]LabeledMetric{"asset-1": {{Metric: "metric", Value: 1}}},
				metas:     map[string]AssetMeta{"asset-1": {Name: "bad\x00name"}},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if metrics := collectAll(NewCollector(tc.src)); len(metrics) != 0 {
				t.Fatalf("invalid scrape emitted %d metrics", len(metrics))
			}
			rr := httptest.NewRecorder()
			NewHandler(tc.src).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/metrics", nil))
			if rr.Code != http.StatusOK || strings.Contains(rr.Body.String(), "labtether_") {
				t.Fatalf("invalid HTTP scrape was not empty/fail-closed: status=%d body=%s", rr.Code, rr.Body.String())
			}
		})
	}
}

func TestCollectorFailsClosedBeforeEmissionWhenExportBudgetExceeded(t *testing.T) {
	const series = 3000
	samples := make([]LabeledMetric, series)
	for i := range samples {
		samples[i] = LabeledMetric{
			Metric: telemetry.MetricProcessCPUPercent,
			Value:  float64(i),
			Labels: map[string]string{"process_pid": time.Unix(int64(i), 0).Format("150405.000000000")},
		}
	}
	src := &mockSource{
		snapshots: map[string][]LabeledMetric{"asset-1": samples},
		metas:     map[string]AssetMeta{"asset-1": {Name: strings.Repeat("n", telemetry.MaxMetricIdentityBytes)}},
	}
	registry := prometheus.NewRegistry()
	if err := registry.Register(NewCollector(src)); err != nil {
		t.Fatalf("register collector: %v", err)
	}
	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("gather oversized scrape: %v", err)
	}
	if len(families) != 0 {
		t.Fatalf("oversized scrape emitted partial metric families: %d", len(families))
	}
}

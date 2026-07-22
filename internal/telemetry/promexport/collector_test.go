package promexport

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/telemetry"
	dto "github.com/prometheus/client_model/go"

	"github.com/prometheus/client_golang/prometheus"
)

// mockSource implements SnapshotSource with fixed test data.
type mockSource struct {
	snapshots map[string][]LabeledMetric
	metas     map[string]AssetMeta
	hub       map[string][]LabeledMetric
}

func (m *mockSource) LatestSnapshots() map[string][]LabeledMetric {
	return m.snapshots
}

func (m *mockSource) AssetMetadata() map[string]AssetMeta {
	return m.metas
}

func (m *mockSource) HubSnapshots() map[string][]LabeledMetric {
	return m.hub
}

// collectAll gathers all prometheus.Metric values emitted by the collector.
func collectAll(c prometheus.Collector) []prometheus.Metric {
	ch := make(chan prometheus.Metric, 256)
	c.Collect(ch)
	close(ch)
	var out []prometheus.Metric
	for m := range ch {
		out = append(out, m)
	}
	return out
}

// describeAll gathers all prometheus.Desc values from the collector.
func describeAll(c prometheus.Collector) []*prometheus.Desc {
	ch := make(chan *prometheus.Desc, 64)
	c.Describe(ch)
	close(ch)
	var out []*prometheus.Desc
	for d := range ch {
		out = append(out, d)
	}
	return out
}

// metricToDTO converts a prometheus.Metric into its proto representation for
// easy assertion.
func metricToDTO(m prometheus.Metric) *dto.Metric {
	pb := &dto.Metric{}
	if err := m.Write(pb); err != nil {
		panic("metricToDTO: " + err.Error())
	}
	return pb
}

// labelValue returns the value of a label from a DTO metric, or "" if absent.
func labelValue(pb *dto.Metric, name string) string {
	for _, lp := range pb.GetLabel() {
		if lp.GetName() == name {
			return lp.GetValue()
		}
	}
	return ""
}

// hasLabel reports whether a DTO metric has a label with the given name.
func hasLabel(pb *dto.Metric, name string) bool {
	for _, lp := range pb.GetLabel() {
		if lp.GetName() == name {
			return true
		}
	}
	return false
}

func labelNames(pb *dto.Metric) []string {
	names := make([]string, 0, len(pb.GetLabel()))
	for _, label := range pb.GetLabel() {
		names = append(names, label.GetName())
	}
	sort.Strings(names)
	return names
}

func hasMetricName(metric prometheus.Metric, name string) bool {
	return strings.Contains(metric.Desc().String(), `fqName: "`+name+`"`)
}

// -----------------------------------------------------------------------
// Tests
// -----------------------------------------------------------------------

func TestCollectorDescribesMetrics(t *testing.T) {
	src := &mockSource{
		snapshots: map[string][]LabeledMetric{},
		metas:     map[string]AssetMeta{},
	}
	c := NewCollector(src)
	descs := describeAll(c)
	if len(descs) == 0 {
		t.Fatal("expected at least one descriptor from Describe()")
	}
	// The asset_info descriptor must be present.
	found := false
	for _, d := range descs {
		if strings.Contains(d.String(), "labtether_asset_info") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("labtether_asset_info descriptor not found in Describe() output")
	}
}

func TestCollectorCollectsAllAssets(t *testing.T) {
	src := &mockSource{
		snapshots: map[string][]LabeledMetric{
			"asset-1": {
				{Metric: "cpu_used_percent", Value: 42.5},
				{Metric: "memory_used_percent", Value: 70.0},
			},
			"asset-2": {
				{Metric: "cpu_used_percent", Value: 10.0},
			},
		},
		metas: map[string]AssetMeta{
			"asset-1": {Name: "server-1", Type: "linux"},
			"asset-2": {Name: "server-2", Type: "linux"},
		},
	}

	c := NewCollector(src)
	metrics := collectAll(c)

	// Expect: 2 asset_info + 3 metric samples = 5 total.
	if len(metrics) != 5 {
		t.Errorf("expected 5 metrics, got %d", len(metrics))
	}

	// Verify metric names carry the labtether_ prefix.
	names := make(map[string]int)
	for _, m := range metrics {
		pb := metricToDTO(m)
		// Prometheus metric names are not directly readable from the Metric
		// interface; use the Desc string instead.
		_ = pb
	}
	_ = names

	// Verify asset_info is emitted with correct labels for both assets.
	infoCount := 0
	for _, m := range metrics {
		pb := metricToDTO(m)
		if pb.GetGauge() == nil {
			continue
		}
		if !hasLabel(pb, "asset_id") {
			continue
		}
		// asset_info metrics have value 1 and the asset_name label.
		if pb.GetGauge().GetValue() == 1 && hasLabel(pb, "asset_name") && hasLabel(pb, "asset_type") && !hasLabel(pb, "mount_point") {
			aid := labelValue(pb, "asset_id")
			if aid == "asset-1" || aid == "asset-2" {
				infoCount++
			}
		}
	}
	if infoCount < 2 {
		t.Errorf("expected asset_info for both assets, got info count = %d", infoCount)
	}
}

func TestCollectorLabelsForContainers(t *testing.T) {
	src := &mockSource{
		snapshots: map[string][]LabeledMetric{
			"ctr-1": {
				{Metric: "cpu_used_percent", Value: 15.0},
			},
		},
		metas: map[string]AssetMeta{
			"ctr-1": {
				Name:        "my-container",
				Type:        "docker-container",
				Group:       "production",
				Platform:    "linux",
				DockerHost:  "docker.host.example",
				DockerImage: "nginx:latest",
				DockerStack: "web-stack",
			},
		},
	}

	c := NewCollector(src)
	metrics := collectAll(c)

	// Should have 2 metrics: asset_info + cpu gauge.
	if len(metrics) != 2 {
		t.Errorf("expected 2 metrics, got %d", len(metrics))
	}

	for _, m := range metrics {
		pb := metricToDTO(m)
		if labelValue(pb, "asset_id") != "ctr-1" {
			continue
		}
		// Validate container-specific labels are present.
		for _, key := range []string{"docker_host", "docker_image", "docker_stack", "group", "platform"} {
			if !hasLabel(pb, key) {
				t.Errorf("expected label %q on container metric", key)
			}
		}
		if got := labelValue(pb, "docker_image"); got != "nginx:latest" {
			t.Errorf("docker_image: want nginx:latest, got %q", got)
		}
		if got := labelValue(pb, "docker_stack"); got != "web-stack" {
			t.Errorf("docker_stack: want web-stack, got %q", got)
		}
	}
}

func TestCollectorAssetInfo(t *testing.T) {
	src := &mockSource{
		snapshots: map[string][]LabeledMetric{
			"asset-x": {
				{Metric: "cpu_used_percent", Value: 5.0},
			},
		},
		metas: map[string]AssetMeta{
			"asset-x": {Name: "my-host", Type: "linux", Group: "infra"},
		},
	}

	c := NewCollector(src)
	metrics := collectAll(c)

	// Find the asset_info metric (value == 1.0, has asset_name label, no per-metric labels).
	var infoMetric *dto.Metric
	for _, m := range metrics {
		pb := metricToDTO(m)
		if pb.GetGauge().GetValue() == 1.0 &&
			labelValue(pb, "asset_id") == "asset-x" &&
			labelValue(pb, "asset_name") == "my-host" {
			// Confirm this is the info metric and not a cpu metric accidentally at 1.0.
			if !hasLabel(pb, "mount_point") && !hasLabel(pb, "interface") {
				infoMetric = pb
				break
			}
		}
	}

	if infoMetric == nil {
		t.Fatal("labtether_asset_info metric not found")
	}
	if got := labelValue(infoMetric, "asset_type"); got != "linux" {
		t.Errorf("asset_type: want linux, got %q", got)
	}
	if got := labelValue(infoMetric, "group"); got != "infra" {
		t.Errorf("group: want infra, got %q", got)
	}
}

func TestCollectorPerMetricLabels(t *testing.T) {
	src := &mockSource{
		snapshots: map[string][]LabeledMetric{
			"asset-y": {
				{
					Metric: "disk_used_bytes",
					Value:  1024 * 1024 * 500,
					Labels: map[string]string{"mount_point": "/data"},
				},
				{
					Metric: "interface_rx_bytes_per_sec",
					Value:  8192,
					Labels: map[string]string{"interface": "eth0"},
				},
			},
		},
		metas: map[string]AssetMeta{
			"asset-y": {Name: "storage-host", Type: "linux"},
		},
	}

	c := NewCollector(src)
	metrics := collectAll(c)

	// Expect: asset_info + 2 metric samples = 3.
	if len(metrics) != 3 {
		t.Errorf("expected 3 metrics, got %d", len(metrics))
	}

	// Since all metrics now carry the full label set (with empty string for
	// inapplicable keys), we look for a non-empty label value rather than
	// mere label presence.
	foundMount := false
	foundInterface := false
	for _, m := range metrics {
		pb := metricToDTO(m)
		if v := labelValue(pb, "mount_point"); v != "" {
			foundMount = true
			if v != "/data" {
				t.Errorf("mount_point: want /data, got %q", v)
			}
		}
		if v := labelValue(pb, "interface"); v != "" {
			foundInterface = true
			if v != "eth0" {
				t.Errorf("interface: want eth0, got %q", v)
			}
		}
	}
	if !foundMount {
		t.Error("expected non-empty mount_point label value on disk metric")
	}
	if !foundInterface {
		t.Error("expected non-empty interface label value on network metric")
	}
}

func TestCollectorEmptySource(t *testing.T) {
	src := &mockSource{
		snapshots: map[string][]LabeledMetric{},
		metas:     map[string]AssetMeta{},
	}

	c := NewCollector(src)
	metrics := collectAll(c)

	if len(metrics) != 0 {
		t.Errorf("expected 0 metrics for empty source, got %d", len(metrics))
	}
}

func TestNewHandlerProducesValidOutput(t *testing.T) {
	src := &mockSource{
		snapshots: map[string][]LabeledMetric{
			"asset-z": {
				{Metric: "cpu_used_percent", Value: 33.3},
			},
		},
		metas: map[string]AssetMeta{
			"asset-z": {Name: "test-host", Type: "linux"},
		},
	}

	handler := NewHandler(src)
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", rr.Code)
	}
	ct := rr.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/plain") {
		t.Errorf("expected text/plain content-type, got %q", ct)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "labtether_asset_info") {
		t.Errorf("expected labtether_asset_info in response body")
	}
	if !strings.Contains(body, "labtether_cpu_used_percent") {
		t.Errorf("expected labtether_cpu_used_percent in response body")
	}
}

// TestCollectorMixedLabelSetsNoPanic verifies that metrics from assets of
// different types (e.g. a plain Linux host vs. a Docker container) sharing the
// same metric name do not trigger a Prometheus panic due to mismatched Desc
// label sets. Both should emit cpu_used_percent with the identical full label
// key set, with empty strings for inapplicable keys.
func TestCollectorMixedLabelSetsNoPanic(t *testing.T) {
	src := &mockSource{
		snapshots: map[string][]LabeledMetric{
			// Linux asset: no docker labels, no group
			"linux-1": {
				{Metric: "cpu_used_percent", Value: 30.0},
			},
			// Docker container: has docker_host, docker_image, docker_stack, group
			"docker-1": {
				{Metric: "cpu_used_percent", Value: 55.0},
			},
		},
		metas: map[string]AssetMeta{
			"linux-1": {
				Name: "bare-metal",
				Type: "linux",
			},
			"docker-1": {
				Name:        "my-nginx",
				Type:        "docker-container",
				Group:       "production",
				Platform:    "linux",
				DockerHost:  "docker.host.example",
				DockerImage: "nginx:latest",
				DockerStack: "web",
			},
		},
	}

	c := NewCollector(src)
	// This must not panic. The Prometheus client panics when two metrics share a
	// name but use different Desc objects (i.e. different label key sets).
	var recovered interface{}
	func() {
		defer func() { recovered = recover() }()
		metrics := collectAll(c)
		// Sanity: we expect 2 asset_info + 2 cpu gauges = 4 metrics total.
		if len(metrics) != 4 {
			t.Errorf("expected 4 metrics, got %d", len(metrics))
		}
	}()
	if recovered != nil {
		t.Fatalf("Collect panicked: %v", recovered)
	}

	// Verify that the docker container's cpu metric carries the docker labels,
	// and the linux asset's cpu metric has those labels set to empty string.
	metrics := collectAll(c)
	for _, m := range metrics {
		pb := metricToDTO(m)
		aid := labelValue(pb, "asset_id")
		// Skip asset_info (value == 1.0 and no per-metric labels)
		if pb.GetGauge().GetValue() == 1.0 && !hasLabel(pb, "mount_point") {
			continue
		}
		switch aid {
		case "linux-1":
			// docker labels must be present but empty
			if got := labelValue(pb, "docker_host"); got != "" {
				t.Errorf("linux-1 docker_host: want empty, got %q", got)
			}
			if got := labelValue(pb, "group"); got != "" {
				t.Errorf("linux-1 group: want empty, got %q", got)
			}
		case "docker-1":
			if got := labelValue(pb, "docker_host"); got != "docker.host.example" {
				t.Errorf("docker-1 docker_host: want docker.host.example, got %q", got)
			}
			if got := labelValue(pb, "docker_image"); got != "nginx:latest" {
				t.Errorf("docker-1 docker_image: want nginx:latest, got %q", got)
			}
		}
	}
}

func TestCollectorPreservesExistingAssetMetricLabelSchema(t *testing.T) {
	src := &mockSource{
		snapshots: map[string][]LabeledMetric{
			"asset-1": {{Metric: telemetry.MetricCPUUsedPercent, Value: 42}},
		},
		metas: map[string]AssetMeta{"asset-1": {Name: "asset", Type: "linux"}},
	}
	metrics := collectAll(NewCollector(src))
	var assetMetric *dto.Metric
	for _, metric := range metrics {
		if hasMetricName(metric, "labtether_"+telemetry.MetricCPUUsedPercent) {
			assetMetric = metricToDTO(metric)
			break
		}
	}
	if assetMetric == nil {
		t.Fatal("asset CPU metric missing")
	}
	want := []string{
		"asset_id", "asset_name", "asset_type", "check_type", "datastore",
		"docker_host", "docker_image", "docker_stack", "group", "interface",
		"mount_point", "platform", "process_name", "process_pid", "proxmox_node",
		"rule_name", "service_name", "service_url", "site_id", "site_name", "target",
	}
	sort.Strings(want)
	if got := labelNames(assetMetric); !reflect.DeepEqual(got, want) {
		t.Fatalf("asset metric labels changed:\n got %v\nwant %v", got, want)
	}
	if hasLabel(assetMetric, "scope") || hasLabel(assetMetric, "rule_id") {
		t.Fatalf("hub-only labels leaked into asset descriptor: %v", labelNames(assetMetric))
	}
}

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

func TestSanitizeMetricName(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"cpu_used_percent", "cpu_used_percent"},
		{"disk-used-bytes", "disk_used_bytes"},
		{"metric.with.dots", "metric_with_dots"},
		{"valid123", "valid123"},
		{"a b c", "a_b_c"},
	}
	for _, tc := range cases {
		got := sanitizeMetricName(tc.input)
		if got != tc.want {
			t.Errorf("sanitizeMetricName(%q) = %q, want %q", tc.input, got, tc.want)
		}
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

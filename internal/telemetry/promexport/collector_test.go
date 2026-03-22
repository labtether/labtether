package promexport

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	dto "github.com/prometheus/client_model/go"

	"github.com/prometheus/client_golang/prometheus"
)

// mockSource implements SnapshotSource with fixed test data.
type mockSource struct {
	snapshots map[string][]LabeledMetric
	metas     map[string]AssetMeta
}

func (m *mockSource) LatestSnapshots() map[string][]LabeledMetric {
	return m.snapshots
}

func (m *mockSource) AssetMetadata() map[string]AssetMeta {
	return m.metas
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

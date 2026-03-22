// Package promexport implements a Prometheus collector and HTTP handler that
// exposes LabTether telemetry in the Prometheus exposition format.
package promexport

import (
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

const metricPrefix = "labtether_"

// AssetMeta holds metadata about an asset used for Prometheus label enrichment.
type AssetMeta struct {
	Name     string
	Type     string // e.g. "linux", "docker-container", "proxmox-vm"
	Group    string
	Platform string
	// Container-specific fields; only set when Type is a container variant.
	DockerHost  string
	DockerImage string
	DockerStack string
}

// LabeledMetric is a single metric sample with its per-sample labels (e.g.
// mount_point, interface) sourced from the metric_samples.labels column.
type LabeledMetric struct {
	Metric string
	Value  float64
	Labels map[string]string
}

// SnapshotSource provides the latest metrics and asset metadata to the
// Prometheus collector on each scrape.
type SnapshotSource interface {
	// LatestSnapshots returns the latest labeled metrics for all assets,
	// keyed by asset ID.
	LatestSnapshots() map[string][]LabeledMetric
	// AssetMetadata returns metadata for label enrichment, keyed by asset ID.
	AssetMetadata() map[string]AssetMeta
}

// perMetricLabels lists the label keys that may appear in a metric sample's
// Labels map and should be forwarded as Prometheus labels on the time series.
var perMetricLabels = []string{
	"mount_point",
	"interface",
	"process_name",
	"process_pid",
	"service_name",
	"service_url",
	"check_type",
	"target",
	"datastore",
	"proxmox_node",
	"site_id",
	"site_name",
	"rule_name",
}

// allMetricLabelKeys is the full, ordered set of label keys used for every
// per-metric gauge. Keeping this fixed ensures every instance of a given metric
// name uses the exact same prometheus.Desc, which is required by the Prometheus
// client library — mismatched Desc objects for the same metric name cause a panic.
//
// Asset-level keys come first (stable), followed by the per-sample label keys
// (also stable). Values that are not applicable to a particular asset or sample
// are set to the empty string so the cardinality stays constant.
var allMetricLabelKeys = func() []string {
	base := []string{
		"asset_id",
		"asset_name",
		"asset_type",
		"group",
		"platform",
		"docker_host",
		"docker_image",
		"docker_stack",
	}
	keys := make([]string, len(base), len(base)+len(perMetricLabels))
	copy(keys, base)
	keys = append(keys, perMetricLabels...)
	return keys
}()

// descCache caches one prometheus.Desc per sanitized metric name. Because the
// label key set is now fixed (allMetricLabelKeys), the Desc is stable across
// all assets, and we can safely reuse it.
var (
	descCacheMu sync.Mutex
	descCache   = map[string]*prometheus.Desc{}
)

// metricDesc returns the cached prometheus.Desc for the given raw metric name,
// creating it on first use.
func metricDesc(rawMetric string) *prometheus.Desc {
	sanitized := sanitizeMetricName(rawMetric)
	descCacheMu.Lock()
	defer descCacheMu.Unlock()
	if d, ok := descCache[sanitized]; ok {
		return d
	}
	d := prometheus.NewDesc(
		metricPrefix+sanitized,
		"LabTether metric: "+rawMetric,
		allMetricLabelKeys,
		nil,
	)
	descCache[sanitized] = d
	return d
}

// assetInfoDesc is the descriptor for the static labtether_asset_info gauge
// that carries asset metadata as labels and is always emitted at value 1.
var assetInfoDesc = prometheus.NewDesc(
	metricPrefix+"asset_info",
	"Static asset metadata gauge. Always 1. Use for PromQL label joins.",
	[]string{"asset_id", "asset_name", "asset_type", "group", "platform",
		"docker_host", "docker_image", "docker_stack"},
	nil,
)

// Collector implements prometheus.Collector. On each scrape it pulls the latest
// snapshots from the SnapshotSource and emits one Gauge time series per metric
// sample, plus a labtether_asset_info series per asset.
type Collector struct {
	source SnapshotSource
}

// NewCollector returns a new Collector backed by the given SnapshotSource.
func NewCollector(source SnapshotSource) *Collector {
	return &Collector{source: source}
}

// Describe sends descriptors to ch. We send only the static asset_info
// descriptor here; metric descriptors are created dynamically in Collect
// because the full set of metric names is not known at registration time.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- assetInfoDesc
}

// Collect fetches the latest snapshots and emits one Gauge per sample plus
// one labtether_asset_info Gauge per asset.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	snapshots := c.source.LatestSnapshots()
	metas := c.source.AssetMetadata()

	for assetID, samples := range snapshots {
		meta := metas[assetID]

		// --- labtether_asset_info ---
		ch <- prometheus.MustNewConstMetric(
			assetInfoDesc,
			prometheus.GaugeValue,
			1,
			assetID,
			meta.Name,
			meta.Type,
			meta.Group,
			meta.Platform,
			meta.DockerHost,
			meta.DockerImage,
			meta.DockerStack,
		)

		// --- per-metric gauges ---
		for _, s := range samples {
			ch <- buildMetric(assetID, meta, s)
		}
	}
}

// buildMetric constructs a prometheus.Metric for a single LabeledMetric sample.
//
// All metrics use the same full label key set (allMetricLabelKeys) regardless of
// asset type. Labels that are not applicable are set to the empty string. This
// guarantees that the prometheus.Desc is identical for every instance of the
// same metric name, which is required to avoid a Prometheus client panic.
func buildMetric(assetID string, meta AssetMeta, s LabeledMetric) prometheus.Metric {
	// Populate values for the fixed base label keys in declaration order:
	//   asset_id, asset_name, asset_type, group, platform,
	//   docker_host, docker_image, docker_stack
	labelVals := make([]string, 0, len(allMetricLabelKeys))
	labelVals = append(labelVals,
		assetID,
		meta.Name,
		meta.Type,
		meta.Group,
		meta.Platform,
		meta.DockerHost,
		meta.DockerImage,
		meta.DockerStack,
	)

	// Per-metric label keys; use empty string when the key is absent or blank.
	for _, key := range perMetricLabels {
		labelVals = append(labelVals, s.Labels[key])
	}

	desc := metricDesc(s.Metric)
	return prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, s.Value, labelVals...)
}

// NoopSnapshotSource is a SnapshotSource that always returns empty data.
// It is used as a placeholder until the real telemetry store implementation
// is wired in (Task 9).
type NoopSnapshotSource struct{}

func (NoopSnapshotSource) LatestSnapshots() map[string][]LabeledMetric { return nil }
func (NoopSnapshotSource) AssetMetadata() map[string]AssetMeta         { return nil }

// sanitizeMetricName replaces characters that are invalid in Prometheus metric
// names (anything that is not [a-zA-Z0-9_]) with underscores.
func sanitizeMetricName(name string) string {
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			return r
		}
		return '_'
	}, name)
}

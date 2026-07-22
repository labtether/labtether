// Package promexport implements a Prometheus collector and HTTP handler that
// exposes LabTether telemetry in the Prometheus exposition format.
package promexport

import (
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/labtether/labtether/internal/telemetry"
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
	Metric      string
	Value       float64
	Labels      map[string]string
	CollectedAt time.Time
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

// HubSnapshotSource is an optional extension for non-asset hub gauges. Keeping
// this channel separate prevents any user-controlled asset ID from colliding
// with or overwriting global telemetry presentation state.
type HubSnapshotSource interface {
	HubSnapshots() map[string][]LabeledMetric
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

var (
	hubDescCacheMu sync.Mutex
	hubDescCache   = map[string]*prometheus.Desc{}
)

// metricDesc returns a scrape-local descriptor. Dynamic metric names are
// agent-controlled, so retaining them in a process-global cache would allow
// permanent memory growth across rotating scrapes.
func metricDesc(cache map[string]*prometheus.Desc, rawMetric string) (*prometheus.Desc, bool) {
	sanitized := sanitizeMetricName(rawMetric)
	if d, ok := cache[sanitized]; ok {
		return d, true
	}
	if len(cache) >= telemetry.MaxPrometheusAssetMetricSeries {
		return nil, false
	}
	d := prometheus.NewDesc(
		metricPrefix+sanitized,
		"LabTether metric: "+sanitized,
		allMetricLabelKeys,
		nil,
	)
	cache[sanitized] = d
	return d, true
}

// hubMetricDesc returns a dedicated descriptor while retaining the established
// labtether_<metric> public names used by bundled dashboards. Reserved hub
// names are filtered from the asset channel, so its distinct labels cannot
// conflict with these closed-schema descriptors.
func hubMetricDesc(scope, rawMetric string) (*prometheus.Desc, bool) {
	scope = strings.TrimSpace(scope)
	rawMetric = strings.TrimSpace(rawMetric)
	metricLabels, ok := telemetry.HubMetricLabelKeys(scope, rawMetric)
	if !ok {
		return nil, false
	}
	sanitized := sanitizeMetricName(rawMetric)
	hubDescCacheMu.Lock()
	defer hubDescCacheMu.Unlock()
	if d, exists := hubDescCache[sanitized]; exists {
		return d, true
	}
	labelKeys := make([]string, 1, len(metricLabels)+1)
	labelKeys[0] = "scope"
	labelKeys = append(labelKeys, metricLabels...)
	d := prometheus.NewDesc(
		metricPrefix+sanitized,
		"LabTether hub metric: "+rawMetric,
		labelKeys,
		nil,
	)
	hubDescCache[sanitized] = d
	return d, true
}

func isReservedPromMetricName(rawMetric string) bool {
	sanitized := sanitizeMetricName(rawMetric)
	for _, metric := range []string{
		"asset_info",
		telemetry.MetricAlertsFiring,
		telemetry.MetricAlertsRules,
		telemetry.MetricAlertEvaluationDurationMs,
		telemetry.MetricSiteReliabilityScore,
		telemetry.MetricSyntheticLatencyMs,
		telemetry.MetricSyntheticStatus,
	} {
		if sanitized == sanitizeMetricName(metric) {
			return true
		}
	}
	return false
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
	type preparedAsset struct {
		assetID string
		meta    AssetMeta
		samples []LabeledMetric
	}
	type preparedHub struct {
		scope   string
		samples []LabeledMetric
	}

	if len(snapshots) > telemetry.MaxPrometheusSnapshotAssets {
		return
	}
	assetIDs := make([]string, 0, len(snapshots))
	for assetID := range snapshots {
		assetIDs = append(assetIDs, assetID)
	}
	sort.Strings(assetIDs)
	preparedAssets := make([]preparedAsset, 0, len(assetIDs))
	exportBytes := 0
	rawAssetSamples := 0
	for _, assetID := range assetIDs {
		samples := snapshots[assetID]
		rawAssetSamples += len(samples)
		if rawAssetSamples > telemetry.MaxPrometheusAssetMetricSeries {
			return
		}
		meta := metas[assetID]
		if !validAssetExportMeta(assetID, meta) {
			return
		}
		for _, sample := range samples {
			if sample.Metric == "" {
				return
			}
			if _, err := telemetry.MetricSampleEnvelopeBytes(telemetry.MetricSample{
				AssetID: assetID,
				Metric:  sample.Metric,
				Unit:    "export",
				Value:   sample.Value,
				Labels:  sample.Labels,
			}); err != nil {
				return
			}
		}
		canonical := canonicalAssetMetrics(samples)
		exportBytes += estimatedExportBytes("asset_info", assetID, meta.Name, meta.Type, meta.Group, meta.Platform, meta.DockerHost, meta.DockerImage, meta.DockerStack)
		for _, sample := range canonical {
			values := []string{assetID, meta.Name, meta.Type, meta.Group, meta.Platform, meta.DockerHost, meta.DockerImage, meta.DockerStack}
			for _, key := range perMetricLabels {
				values = append(values, sample.Labels[key])
			}
			exportBytes += estimatedExportBytes(sanitizeMetricName(sample.Metric), values...)
		}
		if exportBytes > telemetry.MaxPrometheusExportBytes {
			return
		}
		preparedAssets = append(preparedAssets, preparedAsset{assetID: assetID, meta: meta, samples: canonical})
	}

	var preparedHubs []preparedHub
	if hubSource, ok := c.source.(HubSnapshotSource); ok {
		hubSnapshots := hubSource.HubSnapshots()
		scopes := make([]string, 0, len(hubSnapshots))
		for scope := range hubSnapshots {
			scopes = append(scopes, scope)
		}
		sort.Strings(scopes)
		rawHubSamples := 0
		for _, scope := range scopes {
			samples := hubSnapshots[scope]
			rawHubSamples += len(samples)
			if rawHubSamples > telemetry.MaxHubMetricSnapshotSeries {
				return
			}
			validSamples := make([]LabeledMetric, 0, len(samples))
			for _, sample := range samples {
				labels, err := telemetry.NormalizeHubMetricLabels(scope, sample.Metric, sample.Labels)
				if err != nil {
					continue
				}
				if _, err := telemetry.MetricSampleEnvelopeBytes(telemetry.MetricSample{
					Scope: scope, Metric: sample.Metric, Unit: hubMetricUnit(sample.Metric),
					Value: sample.Value, Labels: labels,
				}); err != nil {
					return
				}
				labelKeys, _ := telemetry.HubMetricLabelKeys(scope, sample.Metric)
				values := []string{scope}
				for _, key := range labelKeys {
					values = append(values, labels[key])
				}
				exportBytes += estimatedExportBytes(sanitizeMetricName(sample.Metric), values...)
				sample.Labels = labels
				validSamples = append(validSamples, sample)
			}
			if exportBytes > telemetry.MaxPrometheusExportBytes {
				return
			}
			preparedHubs = append(preparedHubs, preparedHub{scope: scope, samples: validSamples})
		}
	}

	assetDescCache := make(map[string]*prometheus.Desc)
	for _, asset := range preparedAssets {
		ch <- prometheus.MustNewConstMetric(
			assetInfoDesc,
			prometheus.GaugeValue,
			1,
			asset.assetID,
			asset.meta.Name,
			asset.meta.Type,
			asset.meta.Group,
			asset.meta.Platform,
			asset.meta.DockerHost,
			asset.meta.DockerImage,
			asset.meta.DockerStack,
		)
		for _, sample := range asset.samples {
			if metric, ok := buildMetric(assetDescCache, asset.assetID, asset.meta, sample); ok {
				ch <- metric
			}
		}
	}
	for _, hub := range preparedHubs {
		for _, sample := range hub.samples {
			if metric, valid := buildHubMetric(hub.scope, sample); valid {
				ch <- metric
			}
		}
	}
}

func validAssetExportMeta(assetID string, meta AssetMeta) bool {
	for _, value := range []string{assetID, meta.Name, meta.Type, meta.Group, meta.Platform, meta.DockerHost, meta.DockerImage, meta.DockerStack} {
		if len(value) > telemetry.MaxMetricIdentityBytes || !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') {
			return false
		}
	}
	return true
}

func estimatedExportBytes(metric string, labelValues ...string) int {
	// Prometheus text escaping can expand backslashes/newlines/quotes. Three
	// times the raw bytes plus fixed syntax/help overhead is conservative.
	total := 512 + 3*len(metric)
	for _, value := range labelValues {
		total += 3 * len(value)
	}
	return total
}

func hubMetricUnit(metric string) string {
	switch metric {
	case telemetry.MetricAlertsFiring, telemetry.MetricAlertsRules:
		return "count"
	case telemetry.MetricAlertEvaluationDurationMs:
		return "ms"
	case telemetry.MetricSiteReliabilityScore:
		return "score"
	case telemetry.MetricSyntheticLatencyMs:
		return "ms"
	case telemetry.MetricSyntheticStatus:
		return "status"
	default:
		return ""
	}
}

// canonicalAssetMetrics prevents persisted identities that project to one
// Prometheus name+labelset from emitting duplicate series for the same asset.
// The newest sample wins. Equal timestamps prefer the canonical underscore raw
// name, then lexical raw/full-label JSON. Legitimate exported sub-series (for
// example distinct mount_point values) remain separate.
type assetMetricCandidate struct {
	sample         LabeledMetric
	sanitized      string
	fullLabelsJSON string
}

func canonicalAssetMetrics(samples []LabeledMetric) []LabeledMetric {
	byExportIdentity := make(map[string]assetMetricCandidate, len(samples))
	for _, sample := range samples {
		if isReservedPromMetricName(sample.Metric) {
			continue
		}
		sanitized := sanitizeMetricName(sample.Metric)
		projectedValues := make([]string, len(perMetricLabels))
		for i, key := range perMetricLabels {
			projectedValues[i] = sample.Labels[key]
		}
		projectedJSON, _ := json.Marshal(projectedValues)
		fullLabelsJSON := "{}"
		if len(sample.Labels) > 0 {
			encoded, _ := json.Marshal(sample.Labels)
			fullLabelsJSON = string(encoded)
		}
		identity := sanitized + "\x00" + string(projectedJSON)
		incoming := assetMetricCandidate{sample: sample, sanitized: sanitized, fullLabelsJSON: fullLabelsJSON}
		current, exists := byExportIdentity[identity]
		if !exists || preferredAssetMetricCandidate(incoming, current) {
			byExportIdentity[identity] = incoming
		}
	}
	identities := make([]string, 0, len(byExportIdentity))
	for identity := range byExportIdentity {
		identities = append(identities, identity)
	}
	sort.Strings(identities)
	out := make([]LabeledMetric, 0, len(identities))
	for _, identity := range identities {
		out = append(out, byExportIdentity[identity].sample)
	}
	return out
}

func preferredAssetMetricCandidate(incoming, current assetMetricCandidate) bool {
	if incoming.sample.CollectedAt.After(current.sample.CollectedAt) {
		return true
	}
	if current.sample.CollectedAt.After(incoming.sample.CollectedAt) {
		return false
	}
	incomingCanonical := incoming.sample.Metric == incoming.sanitized
	currentCanonical := current.sample.Metric == current.sanitized
	if incomingCanonical != currentCanonical {
		return incomingCanonical
	}
	if incoming.sample.Metric != current.sample.Metric {
		return incoming.sample.Metric < current.sample.Metric
	}
	return incoming.fullLabelsJSON < current.fullLabelsJSON
}

// buildMetric constructs a prometheus.Metric for a single LabeledMetric sample.
//
// All metrics use the same full label key set (allMetricLabelKeys) regardless of
// asset type. Labels that are not applicable are set to the empty string. This
// guarantees that the prometheus.Desc is identical for every instance of the
// same metric name, which is required to avoid a Prometheus client panic.
func buildMetric(descs map[string]*prometheus.Desc, assetID string, meta AssetMeta, s LabeledMetric) (prometheus.Metric, bool) {
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

	desc, ok := metricDesc(descs, s.Metric)
	if !ok {
		return nil, false
	}
	return prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, s.Value, labelVals...), true
}

func buildHubMetric(scope string, sample LabeledMetric) (prometheus.Metric, bool) {
	scope = strings.TrimSpace(scope)
	sample.Metric = strings.TrimSpace(sample.Metric)
	canonicalLabels, err := telemetry.NormalizeHubMetricLabels(scope, sample.Metric, sample.Labels)
	if err != nil {
		return nil, false
	}
	metricLabelKeys, ok := telemetry.HubMetricLabelKeys(scope, sample.Metric)
	if !ok {
		return nil, false
	}
	desc, ok := hubMetricDesc(scope, sample.Metric)
	if !ok {
		return nil, false
	}
	labelValues := make([]string, 1, len(metricLabelKeys)+1)
	labelValues[0] = strings.TrimSpace(scope)
	for _, key := range metricLabelKeys {
		labelValues = append(labelValues, canonicalLabels[key])
	}
	return prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, sample.Value, labelValues...), true
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

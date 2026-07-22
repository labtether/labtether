package remotewrite

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/telemetry"
)

const metricPrefix = "labtether_"

var assetMetricLabelKeys = []string{
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

// AssetSampleRow is the bounded persistence projection required to create the
// same public metric identity and label schema as the Prometheus scrape path.
type AssetSampleRow struct {
	ID          int64
	AssetID     string
	AssetName   string
	AssetType   string
	Platform    string
	DockerHost  string
	DockerImage string
	DockerStack string
	Metric      string
	Unit        string
	Value       float64
	CollectedAt time.Time
	Labels      map[string]string
}

// HubSampleRow is the closed-schema non-asset telemetry projection.
type HubSampleRow struct {
	ID          int64
	Scope       string
	Metric      string
	Unit        string
	Value       float64
	CollectedAt time.Time
	Labels      map[string]string
}

type sampleCandidate struct {
	sample         SampleWithLabels
	rawMetric      string
	fullLabelsJSON string
	rowID          int64
}

type sampleIdentity struct {
	labels    string
	timestamp int64
}

// BuildBatch converts bounded database rows into remote_write samples. It
// fails closed on malformed persisted data rather than silently advancing the
// durable cursor past data that was never accepted by the receiver.
func BuildBatch(current Cursor, assetRows []AssetSampleRow, hubRows []HubSampleRow) (Batch, error) {
	if !current.valid() || len(assetRows)+len(hubRows) > MaxSamplesPerRequest {
		return Batch{}, fmt.Errorf("remotewrite: invalid persisted replay page")
	}
	next := current
	byIdentity := make(map[sampleIdentity]sampleCandidate, len(assetRows)+len(hubRows))
	previousAssetID := current.AssetSampleID

	for _, row := range assetRows {
		if row.ID <= previousAssetID || row.ID <= 0 {
			return Batch{}, fmt.Errorf("remotewrite: asset replay row is out of order")
		}
		previousAssetID = row.ID
		if row.ID > next.AssetSampleID {
			next.AssetSampleID = row.ID
		}
		sample, err := assetRowSample(row)
		if err != nil {
			return Batch{}, fmt.Errorf("remotewrite: invalid asset replay row %d: %w", row.ID, err)
		}
		if isReservedPromMetricName(row.Metric) {
			continue
		}
		insertCanonicalCandidate(byIdentity, newSampleCandidate(sample, row.Metric, row.Labels, row.ID))
	}

	previousHubID := current.HubSampleID
	for _, row := range hubRows {
		if row.ID <= previousHubID || row.ID <= 0 {
			return Batch{}, fmt.Errorf("remotewrite: hub replay row is out of order")
		}
		previousHubID = row.ID
		if row.ID > next.HubSampleID {
			next.HubSampleID = row.ID
		}
		sample, err := hubRowSample(row)
		if err != nil {
			return Batch{}, fmt.Errorf("remotewrite: invalid hub replay row %d: %w", row.ID, err)
		}
		insertCanonicalCandidate(byIdentity, newSampleCandidate(sample, row.Metric, row.Labels, row.ID))
	}

	identities := make([]sampleIdentity, 0, len(byIdentity))
	for identity := range byIdentity {
		identities = append(identities, identity)
	}
	sort.Slice(identities, func(i, j int) bool {
		if identities[i].labels != identities[j].labels {
			return identities[i].labels < identities[j].labels
		}
		return identities[i].timestamp < identities[j].timestamp
	})
	samples := make([]SampleWithLabels, 0, len(identities))
	for _, identity := range identities {
		samples = append(samples, byIdentity[identity].sample)
	}
	return Batch{Samples: samples, Next: next}, nil
}

func assetRowSample(row AssetSampleRow) (SampleWithLabels, error) {
	metric := strings.TrimSpace(row.Metric)
	if row.ID <= 0 || strings.TrimSpace(row.AssetID) == "" || metric == "" || row.CollectedAt.IsZero() {
		return SampleWithLabels{}, fmt.Errorf("required field is missing")
	}
	if _, err := telemetry.MetricSampleEnvelopeBytes(telemetry.MetricSample{
		AssetID:     row.AssetID,
		Metric:      metric,
		Unit:        row.Unit,
		Value:       row.Value,
		CollectedAt: row.CollectedAt,
		Labels:      row.Labels,
	}); err != nil {
		return SampleWithLabels{}, err
	}
	metadataValues := []string{row.AssetName, row.AssetType, row.Platform}
	if row.AssetType == "docker-container" {
		metadataValues = append(metadataValues, row.DockerHost, row.DockerImage, row.DockerStack)
	}
	for _, value := range metadataValues {
		if len(value) > telemetry.MaxMetricIdentityBytes {
			return SampleWithLabels{}, fmt.Errorf("asset metadata exceeds limit")
		}
	}
	dockerHost, dockerImage, dockerStack := "", "", ""
	if row.AssetType == "docker-container" {
		dockerHost, dockerImage, dockerStack = row.DockerHost, row.DockerImage, row.DockerStack
	}
	labels := map[string]string{
		"__name__":     metricPrefix + sanitizeMetricName(metric),
		"asset_id":     row.AssetID,
		"asset_name":   row.AssetName,
		"asset_type":   row.AssetType,
		"group":        "",
		"platform":     row.Platform,
		"docker_host":  dockerHost,
		"docker_image": dockerImage,
		"docker_stack": dockerStack,
	}
	for _, key := range assetMetricLabelKeys {
		labels[key] = row.Labels[key]
	}
	return SampleWithLabels{Labels: labels, Value: row.Value, Timestamp: row.CollectedAt.UnixMilli()}, nil
}

func hubRowSample(row HubSampleRow) (SampleWithLabels, error) {
	if row.ID <= 0 || row.CollectedAt.IsZero() {
		return SampleWithLabels{}, fmt.Errorf("required field is missing")
	}
	normalized, err := telemetry.NormalizeHubMetricSample(telemetry.MetricSample{
		Scope:       row.Scope,
		Metric:      row.Metric,
		Unit:        row.Unit,
		Value:       row.Value,
		CollectedAt: row.CollectedAt,
		Labels:      row.Labels,
	})
	if err != nil {
		return SampleWithLabels{}, err
	}
	labels := map[string]string{
		"__name__": metricPrefix + sanitizeMetricName(normalized.Metric),
		"scope":    normalized.Scope,
	}
	keys, _ := telemetry.HubMetricLabelKeys(normalized.Scope, normalized.Metric)
	for _, key := range keys {
		labels[key] = normalized.Labels[key]
	}
	return SampleWithLabels{Labels: labels, Value: normalized.Value, Timestamp: normalized.CollectedAt.UnixMilli()}, nil
}

func insertCanonicalCandidate(target map[sampleIdentity]sampleCandidate, incoming sampleCandidate) {
	identity := sampleIdentity{labels: labelFingerprint(incoming.sample.Labels), timestamp: incoming.sample.Timestamp}
	current, exists := target[identity]
	if !exists || preferredCandidate(incoming, current) {
		target[identity] = incoming
	}
}

func newSampleCandidate(sample SampleWithLabels, rawMetric string, rawLabels map[string]string, rowID int64) sampleCandidate {
	fullLabelsJSON := "{}"
	if len(rawLabels) > 0 {
		encoded, _ := json.Marshal(rawLabels)
		fullLabelsJSON = string(encoded)
	}
	return sampleCandidate{sample: sample, rawMetric: rawMetric, fullLabelsJSON: fullLabelsJSON, rowID: rowID}
}

func preferredCandidate(incoming, current sampleCandidate) bool {
	incomingCanonical := incoming.rawMetric == sanitizeMetricName(incoming.rawMetric)
	currentCanonical := current.rawMetric == sanitizeMetricName(current.rawMetric)
	if incomingCanonical != currentCanonical {
		return incomingCanonical
	}
	if incoming.rawMetric != current.rawMetric {
		return incoming.rawMetric < current.rawMetric
	}
	if incoming.fullLabelsJSON != current.fullLabelsJSON {
		return incoming.fullLabelsJSON < current.fullLabelsJSON
	}
	return incoming.rowID > current.rowID
}

func isReservedPromMetricName(rawMetric string) bool {
	sanitized := sanitizeMetricName(rawMetric)
	for _, metric := range []string{
		"asset_info",
		telemetry.MetricAlertsFiring,
		telemetry.MetricAlertsRules,
		telemetry.MetricAlertEvaluationDurationMs,
		telemetry.MetricSiteReliabilityScore,
	} {
		if sanitized == sanitizeMetricName(metric) {
			return true
		}
	}
	return false
}

func sanitizeMetricName(name string) string {
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			return r
		}
		return '_'
	}, name)
}

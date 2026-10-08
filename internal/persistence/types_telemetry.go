package persistence

import (
	"context"
	"errors"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/telemetry"
	"time"
)

// TelemetryStore provides canonical metrics persistence and query paths.
type TelemetryStore interface {
	AppendSamples(ctx context.Context, samples []telemetry.MetricSample) error
	Snapshot(assetID string, at time.Time) (telemetry.Snapshot, error)
	Series(assetID string, start, end time.Time, step time.Duration) ([]telemetry.Series, error)
}

// TelemetrySnapshotBatchStore is an optional optimization interface for
// loading latest metric snapshots for many assets in a single store call.
type TelemetrySnapshotBatchStore interface {
	SnapshotMany(assetIDs []string, at time.Time) (map[string]telemetry.Snapshot, error)
}

// TelemetryDynamicStore is an optional interface for stores that can return
// dynamic (all-metric) snapshots. Callers that type-assert to this interface
// receive a full map of every metric for an asset, not just the 6 canonical ones.
type TelemetryDynamicStore interface {
	DynamicSnapshotForAsset(assetID string, at time.Time) (telemetry.DynamicSnapshot, error)
	DynamicSnapshotMany(assetIDs []string, at time.Time) (map[string]telemetry.DynamicSnapshot, error)
}

// TelemetryLabeledSnapshotStore is the cancellation-aware, cardinality-bounded
// Prometheus path. It preserves the latest sample for every distinct
// (asset, raw metric, labels) series rather than collapsing labeled sub-series
// into a single value per raw metric.
type TelemetryLabeledSnapshotStore interface {
	LatestLabeledMetricSnapshots(ctx context.Context, assetIDs []string, at time.Time, maxSeries int) (map[string][]telemetry.MetricSample, error)
}

// TelemetryHubMetricStore exposes persisted non-asset hub gauges. These
// samples never appear in AssetStore and are merged only into observability
// outputs such as the Prometheus scrape.
type TelemetryHubMetricStore interface {
	HubMetricSnapshots(ctx context.Context, at time.Time, maxSeries int) (map[string][]telemetry.MetricSample, error)
}

// AlertRuleMetricSnapshot is the latest evaluation timing for one active rule.
type AlertRuleMetricSnapshot struct {
	RuleID     string
	RuleName   string
	DurationMS int
}

// AlertMetricsSnapshot combines an exact aggregate rule count with a bounded
// list of per-rule timing series.
type AlertMetricsSnapshot struct {
	ActiveRuleCount     int64
	FiringInstanceCount int64
	RuleEvaluations     []AlertRuleMetricSnapshot
}

// AlertMetricsSnapshotStore avoids a capped list plus one evaluation query per
// rule in the Prometheus bridge.
type AlertMetricsSnapshotStore interface {
	AlertMetricsSnapshot(ctx context.Context, maxRuleSeries int) (AlertMetricsSnapshot, error)
}

// ReliabilityMetricSnapshot is the latest materialized reliability score for
// one group, including the stable identity labels required by Prometheus.
type ReliabilityMetricSnapshot struct {
	GroupID   string
	GroupName string
	Score     int
}

// ReliabilityMetricSnapshotStore loads latest-per-group reliability values in
// one deterministic, cancellation-aware, cardinality-bounded operation.
type ReliabilityMetricSnapshotStore interface {
	LatestReliabilityMetricSnapshots(ctx context.Context, at time.Time, maxGroups int) ([]ReliabilityMetricSnapshot, error)
}

// TelemetryAlertBatchStore is an optional optimization interface for alert
// evaluation paths that only need one metric or simple sample presence checks.
type TelemetryAlertBatchStore interface {
	MetricSeriesBatch(assetIDs []string, metric string, start, end time.Time, step time.Duration) (map[string]telemetry.Series, error)
	AssetsWithSamples(assetIDs []string, start, end time.Time) (map[string]bool, error)
}

// ErrTelemetryQueryLimitExceeded means a caller-visible telemetry query would
// require more raw rows than its explicit memory/work budget permits.
var ErrTelemetryQueryLimitExceeded = errors.New("telemetry query row limit exceeded")

// ErrHubMetricSnapshotLimitExceeded means the persisted hub-scope cardinality
// exceeded the explicit scrape work budget.
var ErrHubMetricSnapshotLimitExceeded = errors.New("hub metric snapshot series limit exceeded")

// ErrAlertMetricSnapshotLimitExceeded means a caller requested more per-rule
// alert series than the bounded observability snapshot permits.
var ErrAlertMetricSnapshotLimitExceeded = errors.New("alert metric snapshot series limit exceeded")

// ErrReliabilityMetricSnapshotLimitExceeded means a caller requested more
// group reliability series than the bounded observability snapshot permits.
var ErrReliabilityMetricSnapshotLimitExceeded = errors.New("reliability metric snapshot series limit exceeded")

var (
	ErrTelemetrySnapshotAssetLimitExceeded  = errors.New("telemetry snapshot asset limit exceeded")
	ErrTelemetrySnapshotRowLimitExceeded    = errors.New("telemetry snapshot raw row limit exceeded")
	ErrTelemetrySnapshotSeriesLimitExceeded = errors.New("telemetry snapshot series limit exceeded")
)

var (
	ErrMetricSampleBatchLimitExceeded = errors.New("metric sample batch count limit exceeded")
	ErrMetricSampleBatchBytesExceeded = errors.New("metric sample batch byte limit exceeded")
)

// TelemetryQueryBatchStore is the cancellation-aware, work-bounded batch path
// used by interactive/API queries. Background alert evaluation intentionally
// retains the narrower TelemetryAlertBatchStore contract above.
type TelemetryQueryBatchStore interface {
	MetricSeriesBatchContext(ctx context.Context, assetIDs []string, metric string, start, end time.Time, step time.Duration, maxRawPoints int) (map[string]telemetry.Series, error)
}

// LogStore provides normalized log/event persistence and query paths.
type LogStore interface {
	AppendEvent(event logs.Event) error
	QueryEvents(req logs.QueryRequest) ([]logs.Event, error)
	ListSources(limit int) ([]logs.SourceSummary, error)
	SaveView(actorID string, req logs.SavedViewRequest) (logs.SavedView, error)
	ListViews(actorID string, limit int) ([]logs.SavedView, error)
	GetView(actorID, id string) (logs.SavedView, bool, error)
	UpdateView(actorID, id string, req logs.SavedViewRequest) (logs.SavedView, error)
	DeleteView(actorID, id string) error
}

// LogBatchAppendStore is an optional optimization interface for appending
// many log events in one store call.
type LogBatchAppendStore interface {
	AppendEvents(events []logs.Event) error
}

// LogSourceSummaryStore is an optional optimization interface for exact source
// aggregations over a filtered log window without materializing raw events.
type LogSourceSummaryStore interface {
	QuerySourceSummaries(req logs.SourceSummaryRequest) ([]logs.SourceSummary, error)
}

// LogGroupSeverityCountStore is an optional optimization interface for exact
// per-group severity aggregations over a filtered log window without
// materializing raw log events.
type LogGroupSeverityCountStore interface {
	QueryGroupSeverityCounts(req logs.GroupSeverityCountRequest) ([]logs.GroupSeverityCount, error)
}

// DeadLetterLogStore is an optional optimization interface for dead-letter
// list/analytics query paths that do not require full log event payloads.
type DeadLetterLogStore interface {
	QueryDeadLetterEvents(from, to time.Time, limit int) ([]logs.DeadLetterEvent, error)
}

// DeadLetterLogCountStore is an optional optimization interface for obtaining
// exact dead-letter totals within a time range without fetching event payloads.
type DeadLetterLogCountStore interface {
	CountDeadLetterEvents(from, to time.Time) (int, error)
}

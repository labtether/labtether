package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/metricschema"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/telemetry"
)

const (
	maxMetricsQueryAssets    = 64
	maxMetricsQueryAssetLen  = 255
	maxMetricsQueryWindow    = 30 * 24 * time.Hour
	maxMetricsQueryPoints    = 2000
	maxMetricsQueryRawPoints = 200_000
	metricsQueryTimeout      = 15 * time.Second
	metricsQueryRateLimit    = 120
)

func (s *apiServer) handleV2MetricsOverview(w http.ResponseWriter, r *http.Request) {
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "metrics:read") {
		apiv2.WriteScopeForbidden(w, "metrics:read")
		return
	}
	r.URL.Path = "/metrics/overview"
	s.handleMetricsOverview(w, r)
}

// handleV2MetricsQuery serves GET /api/v2/metrics/query.
//
// Query parameters:
//
//	asset_ids  – comma-separated asset IDs (required)
//	metric     – metric name (required)
//	from       – RFC 3339 start time (optional, default now-1h)
//	to         – RFC 3339 end time   (optional, default now)
//	step       – duration string, e.g. "1m" (optional, default auto)
func (s *apiServer) handleV2MetricsQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apiv2.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET required")
		return
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "metrics:read") {
		apiv2.WriteScopeForbidden(w, "metrics:read")
		return
	}
	if s.telemetryStore == nil {
		apiv2.WriteError(w, http.StatusServiceUnavailable, "unavailable", "telemetry store unavailable")
		return
	}
	if !s.enforceRateLimit(w, r, "v2.metrics.query", metricsQueryRateLimit, time.Minute) {
		return
	}

	q := r.URL.Query()

	// Parse asset_ids.
	rawIDs := strings.TrimSpace(q.Get("asset_ids"))
	if rawIDs == "" {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "asset_ids is required")
		return
	}
	parts := strings.Split(rawIDs, ",")
	if len(parts) > maxMetricsQueryAssets {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "too many asset_ids")
		return
	}
	assetIDs := make([]string, 0, len(parts))
	seenAssetIDs := make(map[string]struct{}, len(parts))
	for _, p := range parts {
		if id := strings.TrimSpace(p); id != "" {
			if len(id) > maxMetricsQueryAssetLen {
				apiv2.WriteError(w, http.StatusBadRequest, "validation", "asset_id is too long")
				return
			}
			if _, exists := seenAssetIDs[id]; exists {
				continue
			}
			seenAssetIDs[id] = struct{}{}
			assetIDs = append(assetIDs, id)
		}
	}
	if len(assetIDs) == 0 {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "asset_ids must not be empty")
		return
	}

	// Filter by asset allowlist.
	allowed := allowedAssetsFromContext(r.Context())
	filtered := make([]string, 0, len(assetIDs))
	for _, id := range assetIDs {
		if apiv2.AssetCheck(allowed, id) {
			filtered = append(filtered, id)
		}
	}
	if len(filtered) == 0 {
		apiv2.WriteError(w, http.StatusForbidden, "asset_forbidden", "none of the requested assets are accessible with this API key")
		return
	}

	// Parse metric name.
	rawMetric := strings.TrimSpace(q.Get("metric"))
	if rawMetric == "" {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "metric is required")
		return
	}
	metric := canonicalMetricsQueryMetric(rawMetric)
	if metric == "" {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "unsupported metric")
		return
	}

	// Parse time range.
	now := time.Now().UTC()
	from := now.Add(-time.Hour)
	to := now

	if raw := strings.TrimSpace(q.Get("from")); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			apiv2.WriteError(w, http.StatusBadRequest, "validation", "from must be RFC 3339 (e.g. 2006-01-02T15:04:05Z)")
			return
		}
		from = t.UTC()
	}
	if raw := strings.TrimSpace(q.Get("to")); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			apiv2.WriteError(w, http.StatusBadRequest, "validation", "to must be RFC 3339 (e.g. 2006-01-02T15:04:05Z)")
			return
		}
		to = t.UTC()
	}
	if !from.Before(to) {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "from must be before to")
		return
	}
	if to.Sub(from) > maxMetricsQueryWindow {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "metrics query window exceeds 30 days")
		return
	}

	window := to.Sub(from)
	step := metricsQueryAutoStep(window)
	if raw := strings.TrimSpace(q.Get("step")); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil || d <= 0 {
			apiv2.WriteError(w, http.StatusBadRequest, "validation", "step must be a valid positive duration (e.g. 1m, 5m)")
			return
		}
		step = d
	}
	minimumStep := metricsQueryMinimumStep(window)
	if step < minimumStep {
		step = minimumStep
	}

	queryCtx, cancel := context.WithTimeout(r.Context(), metricsQueryTimeout)
	defer cancel()

	// Prefer the cancellation-aware and work-bounded batch path for API calls.
	if batchStore, ok := s.telemetryStore.(persistence.TelemetryQueryBatchStore); ok {
		results, err := batchStore.MetricSeriesBatchContext(queryCtx, filtered, metric, from, to, step, maxMetricsQueryRawPoints)
		if err != nil {
			if errors.Is(err, persistence.ErrTelemetryQueryLimitExceeded) {
				apiv2.WriteError(w, http.StatusUnprocessableEntity, "query_too_large", "telemetry query exceeds the raw-point budget; use a shorter time range")
				return
			}
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				apiv2.WriteError(w, http.StatusGatewayTimeout, "timeout", "telemetry query timed out or was canceled")
				return
			}
			apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to query telemetry")
			return
		}
		apiv2.WriteJSON(w, http.StatusOK, map[string]any{
			"metric":  metric,
			"from":    from,
			"to":      to,
			"step":    step.String(),
			"results": results,
		})
		return
	}

	// Fallback: query per-asset and extract the requested metric.
	results := make(map[string]any, len(filtered))
	for _, id := range filtered {
		if err := queryCtx.Err(); err != nil {
			apiv2.WriteError(w, http.StatusGatewayTimeout, "timeout", "telemetry query timed out or was canceled")
			return
		}
		series, err := s.telemetryStore.Series(id, from, to, step)
		if err != nil {
			apiv2.WriteError(w, http.StatusInternalServerError, "internal", "failed to query telemetry for asset "+id)
			return
		}
		for _, ser := range series {
			if ser.Metric == metric {
				results[id] = ser
				break
			}
		}
	}
	apiv2.WriteJSON(w, http.StatusOK, map[string]any{
		"metric":  metric,
		"from":    from,
		"to":      to,
		"step":    step.String(),
		"results": results,
	})
}

func canonicalMetricsQueryMetric(metric string) string {
	switch metric {
	case metricschema.HeartbeatKeyCPUPercent:
		metric = telemetry.MetricCPUUsedPercent
	case metricschema.HeartbeatKeyMemoryPercent:
		metric = telemetry.MetricMemoryUsedPercent
	case metricschema.HeartbeatKeyDiskPercent:
		metric = telemetry.MetricDiskUsedPercent
	case metricschema.HeartbeatKeyTempCelsius:
		metric = telemetry.MetricTemperatureCelsius
	}
	for _, definition := range telemetry.CanonicalMetrics() {
		if metric == definition.Metric {
			return metric
		}
	}
	return ""
}

func metricsQueryMinimumStep(window time.Duration) time.Duration {
	if window <= 0 {
		return time.Second
	}
	step := (window + maxMetricsQueryPoints - 1) / maxMetricsQueryPoints
	if step < time.Second {
		return time.Second
	}
	return step
}

// metricsQueryAutoStep chooses a reasonable default step duration for a query window.
func metricsQueryAutoStep(window time.Duration) time.Duration {
	switch {
	case window <= 30*time.Minute:
		return 30 * time.Second
	case window <= 2*time.Hour:
		return time.Minute
	case window <= 6*time.Hour:
		return 5 * time.Minute
	case window <= 24*time.Hour:
		return 10 * time.Minute
	default:
		return 30 * time.Minute
	}
}

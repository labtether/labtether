package main

import (
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/persistence"
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

	q := r.URL.Query()

	// Parse asset_ids.
	rawIDs := strings.TrimSpace(q.Get("asset_ids"))
	if rawIDs == "" {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "asset_ids is required")
		return
	}
	parts := strings.Split(rawIDs, ",")
	assetIDs := make([]string, 0, len(parts))
	for _, p := range parts {
		if id := strings.TrimSpace(p); id != "" {
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
	metric := strings.TrimSpace(q.Get("metric"))
	if metric == "" {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "metric is required")
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

	// Use the optimised batch path when the store supports it.
	if batchStore, ok := s.telemetryStore.(persistence.TelemetryAlertBatchStore); ok {
		results, err := batchStore.MetricSeriesBatch(filtered, metric, from, to, step)
		if err != nil {
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

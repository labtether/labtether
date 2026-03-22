package resources

import (
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/servicehttp"
)

type frontendPerfTelemetryRequest struct {
	Route      string         `json:"route"`
	Metric     string         `json:"metric"`
	DurationMS float64        `json:"duration_ms"`
	Status     string         `json:"status,omitempty"`
	SampleSize int            `json:"sample_size,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

func (d *Deps) HandleFrontendPerfTelemetry(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/telemetry/frontend/perf" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if d.LogStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "log store unavailable")
		return
	}

	var req frontendPerfTelemetryRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid telemetry payload")
		return
	}

	route := strings.ToLower(strings.TrimSpace(req.Route))
	switch route {
	case "dashboard", "logs", "devices", "services", "topology":
	default:
		servicehttp.WriteError(w, http.StatusBadRequest, "route must be one of dashboard, logs, devices, services, topology")
		return
	}

	metric := strings.TrimSpace(req.Metric)
	if metric == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "metric is required")
		return
	}
	if len(metric) > 120 {
		metric = metric[:120]
	}

	durationMS := req.DurationMS
	if math.IsNaN(durationMS) || math.IsInf(durationMS, 0) || durationMS < 0 {
		servicehttp.WriteError(w, http.StatusBadRequest, "duration_ms must be a finite non-negative number")
		return
	}
	if durationMS > 3_600_000 {
		durationMS = 3_600_000
	}

	status := strings.ToLower(strings.TrimSpace(req.Status))
	if len(status) > 40 {
		status = status[:40]
	}

	fields := map[string]string{
		"route":       route,
		"metric":      metric,
		"duration_ms": strconv.FormatFloat(durationMS, 'f', 2, 64),
	}
	if status != "" {
		fields["status"] = status
	}
	if req.SampleSize > 0 {
		if req.SampleSize > 100_000 {
			req.SampleSize = 100_000
		}
		fields["sample_size"] = strconv.Itoa(req.SampleSize)
	}

	for key, value := range NormalizeFrontendPerfMetadata(req.Metadata) {
		fields["meta_"+key] = value
	}

	if err := d.LogStore.AppendEvent(logs.Event{
		Source:    "frontend_perf",
		Level:     "info",
		Message:   "frontend route performance metric",
		Fields:    fields,
		Timestamp: time.Now().UTC(),
	}); err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to persist telemetry event")
		return
	}

	servicehttp.WriteJSON(w, http.StatusAccepted, map[string]any{"accepted": true})
}

func NormalizeFrontendPerfMetadata(input map[string]any) map[string]string {
	if len(input) == 0 {
		return map[string]string{}
	}
	const maxEntries = 16
	out := make(map[string]string, len(input))
	for key, value := range input {
		if len(out) >= maxEntries {
			break
		}
		normalizedKey := SanitizeFrontendPerfMetadataKey(key)
		if normalizedKey == "" {
			continue
		}
		formatted := FormatFrontendPerfMetadataValue(value)
		if formatted == "" {
			continue
		}
		if len(formatted) > 180 {
			formatted = formatted[:180]
		}
		out[normalizedKey] = formatted
	}
	return out
}

func SanitizeFrontendPerfMetadataKey(key string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return ""
	}
	key = strings.NewReplacer(" ", "_", "-", "_", "/", "_").Replace(key)
	builder := strings.Builder{}
	builder.Grow(len(key))
	for _, r := range key {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			builder.WriteRune(r)
		}
	}
	return strings.Trim(builder.String(), "_")
}

func FormatFrontendPerfMetadataValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case bool:
		if typed {
			return "true"
		}
		return "false"
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) {
			return ""
		}
		return strconv.FormatFloat(typed, 'f', 2, 64)
	case float32:
		floatValue := float64(typed)
		if math.IsNaN(floatValue) || math.IsInf(floatValue, 0) {
			return ""
		}
		return strconv.FormatFloat(floatValue, 'f', 2, 64)
	case int:
		return strconv.Itoa(typed)
	case int8:
		return strconv.FormatInt(int64(typed), 10)
	case int16:
		return strconv.FormatInt(int64(typed), 10)
	case int32:
		return strconv.FormatInt(int64(typed), 10)
	case int64:
		return strconv.FormatInt(typed, 10)
	case uint:
		return strconv.FormatUint(uint64(typed), 10)
	case uint8:
		return strconv.FormatUint(uint64(typed), 10)
	case uint16:
		return strconv.FormatUint(uint64(typed), 10)
	case uint32:
		return strconv.FormatUint(uint64(typed), 10)
	case uint64:
		return strconv.FormatUint(typed, 10)
	default:
		return ""
	}
}

package resources

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/servicehttp"
)

const (
	maxMobileTelemetryBatch        = 100
	telemetryIngestRateLimitBucket = "telemetry.ingest"
	telemetryIngestRateLimitCount  = 60
)

type mobileClientTelemetryRequest struct {
	Route      string         `json:"route"`
	Metric     string         `json:"metric"`
	DurationMS float64        `json:"duration_ms,omitempty"`
	Status     string         `json:"status,omitempty"`
	SampleSize int            `json:"sample_size,omitempty"`
	Platform   string         `json:"platform,omitempty"`
	AppVersion string         `json:"app_version,omitempty"`
	Build      string         `json:"build,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type mobileClientTelemetryBatchRequest struct {
	Events []mobileClientTelemetryRequest `json:"events"`
}

func (d *Deps) HandleMobileClientTelemetry(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/telemetry/mobile/client" {
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
	if d.EnforceRateLimit != nil && !d.EnforceRateLimit(w, r, telemetryIngestRateLimitBucket, telemetryIngestRateLimitCount, time.Minute) {
		return
	}

	requests, err := decodeMobileClientTelemetryRequests(r)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid telemetry payload")
		return
	}
	if len(requests) == 0 {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid telemetry payload")
		return
	}
	if len(requests) > maxMobileTelemetryBatch {
		servicehttp.WriteError(w, http.StatusBadRequest, "telemetry batch contains too many events")
		return
	}

	events := make([]logs.Event, 0, len(requests))
	for _, req := range requests {
		route := SanitizeMobileTelemetryKey(req.Route)
		if route == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "route is required")
			return
		}

		metric := normalizeTelemetryText(req.Metric, 120)
		if metric == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "metric is required")
			return
		}

		durationMS := req.DurationMS
		if math.IsNaN(durationMS) || math.IsInf(durationMS, 0) || durationMS < 0 {
			servicehttp.WriteError(w, http.StatusBadRequest, "duration_ms must be a finite non-negative number")
			return
		}
		if durationMS > 3_600_000 {
			durationMS = 3_600_000
		}

		status := normalizeTelemetryText(strings.ToLower(req.Status), 40)

		platform := SanitizeMobileTelemetryKey(req.Platform)
		if platform == "" {
			platform = "ios"
		}

		fields := map[string]string{
			"route":       route,
			"metric":      metric,
			"duration_ms": strconv.FormatFloat(durationMS, 'f', 2, 64),
			"platform":    platform,
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
		if trimmed := normalizeTelemetryText(req.AppVersion, 40); trimmed != "" {
			fields["app_version"] = trimmed
		}
		if trimmed := normalizeTelemetryText(req.Build, 40); trimmed != "" {
			fields["build"] = trimmed
		}

		for key, value := range NormalizeFrontendPerfMetadata(req.Metadata) {
			fields["meta_"+key] = value
		}

		events = append(events, logs.Event{
			Source:    "mobile_client_telemetry",
			Level:     mobileTelemetryLevel(status),
			Message:   "mobile client telemetry metric",
			Fields:    fields,
			Timestamp: time.Now().UTC(),
		})
	}

	var persistErr error
	if batchStore, ok := d.LogStore.(persistence.LogBatchAppendStore); ok {
		persistErr = batchStore.AppendEvents(events)
	} else {
		for _, event := range events {
			if err := d.LogStore.AppendEvent(event); err != nil {
				persistErr = err
				break
			}
		}
	}
	if persistErr != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to persist telemetry event")
		return
	}

	servicehttp.WriteJSON(w, http.StatusAccepted, map[string]any{
		"accepted":       true,
		"accepted_count": len(requests),
	})
}

func mobileTelemetryLevel(status string) string {
	switch status {
	case "error", "critical":
		return "error"
	case "warning", "degraded":
		return "warning"
	default:
		return "info"
	}
}

func SanitizeMobileTelemetryKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}

	var b strings.Builder
	b.Grow(len(value))
	for _, r := range value {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '.' || r == '-':
			b.WriteRune(r)
		case r == ' ' || r == '/':
			b.WriteRune('_')
		}
		if b.Len() >= 80 {
			break
		}
	}
	return strings.Trim(b.String(), "._-")
}

const maxTelemetryBodyBytes = 1 << 20 // 1 MiB — matches shared.MaxJSONBodyBytes

func decodeMobileClientTelemetryRequests(r *http.Request) ([]mobileClientTelemetryRequest, error) {
	defer r.Body.Close()

	body, err := io.ReadAll(io.LimitReader(r.Body, maxTelemetryBodyBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxTelemetryBodyBytes {
		return nil, fmt.Errorf("request body too large")
	}

	var batch mobileClientTelemetryBatchRequest
	if err := json.Unmarshal(body, &batch); err == nil && len(batch.Events) > 0 {
		return batch.Events, nil
	}

	var single mobileClientTelemetryRequest
	if err := json.Unmarshal(body, &single); err == nil {
		return []mobileClientTelemetryRequest{single}, nil
	}

	return nil, io.EOF
}

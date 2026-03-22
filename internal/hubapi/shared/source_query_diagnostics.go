package shared

import (
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

type SourceQueryDiagnosticAggregate struct {
	Scope           string    `json:"scope"`
	Caller          string    `json:"caller"`
	Mode            string    `json:"mode"`
	Calls           int       `json:"calls"`
	CacheHits       int       `json:"cache_hits"`
	Errors          int       `json:"errors"`
	AvgDurationMS   float64   `json:"avg_duration_ms"`
	LastDurationMS  float64   `json:"last_duration_ms"`
	LastStatus      string    `json:"last_status"`
	LastWindowStart time.Time `json:"last_window_start"`
	LastWindowEnd   time.Time `json:"last_window_end"`
	LastAt          time.Time `json:"last_at"`
}

var (
	sourceQueryAggregatesMu sync.Mutex
	sourceQueryAggregates   = map[string]SourceQueryDiagnosticAggregate{}
)

func SourceQueryCaller(r *http.Request, fallback string) string {
	if r == nil {
		return NormalizeSourceQueryCaller(fallback, fallback)
	}

	caller := strings.TrimSpace(r.URL.Query().Get("caller"))
	if caller == "" {
		caller = strings.TrimSpace(r.Header.Get("X-Labtether-Caller"))
	}
	return NormalizeSourceQueryCaller(caller, fallback)
}

func NormalizeSourceQueryCaller(raw, fallback string) string {
	caller := strings.TrimSpace(raw)
	if caller == "" {
		caller = strings.TrimSpace(fallback)
	}
	if caller == "" {
		caller = "unknown"
	}
	if len(caller) > 80 {
		caller = caller[:80]
	}
	return caller
}

func SourceQueryDiagnosticsEnabled() bool {
	return EnvOrDefaultBool("DEV_MODE", false) || EnvOrDefaultBool("LABTETHER_DEBUG_LOG_SOURCES", false)
}

func LogSourceQueryDiagnostic(
	scope string,
	caller string,
	mode string,
	groupFiltered bool,
	cacheHit bool,
	windowStart time.Time,
	windowEnd time.Time,
	limit int,
	sourceCount int,
	startedAt time.Time,
	err error,
) {
	if !SourceQueryDiagnosticsEnabled() {
		return
	}

	durationMS := float64(time.Since(startedAt).Microseconds()) / 1000.0
	status := "ok"
	errMessage := ""
	if err != nil {
		status = "error"
		errMessage = strings.TrimSpace(err.Error())
	}
	RecordSourceQueryDiagnosticAggregate(
		strings.TrimSpace(scope),
		NormalizeSourceQueryCaller(caller, "unknown"),
		strings.TrimSpace(mode),
		cacheHit,
		windowStart,
		windowEnd,
		durationMS,
		status,
	)

	log.Printf(
		"source query diag: scope=%s caller=%s mode=%s group_filtered=%t cache_hit=%t window_start=%s window_end=%s limit=%d source_count=%d duration_ms=%.2f status=%s err=%q",
		strings.TrimSpace(scope),
		NormalizeSourceQueryCaller(caller, "unknown"),
		strings.TrimSpace(mode),
		groupFiltered,
		cacheHit,
		windowStart.UTC().Format(time.RFC3339),
		windowEnd.UTC().Format(time.RFC3339),
		limit,
		sourceCount,
		durationMS,
		status,
		errMessage,
	)
}

func RecordSourceQueryDiagnosticAggregate(
	scope string,
	caller string,
	mode string,
	cacheHit bool,
	windowStart time.Time,
	windowEnd time.Time,
	durationMS float64,
	status string,
) {
	key := strings.Join([]string{scope, caller, mode}, "|")

	sourceQueryAggregatesMu.Lock()
	entry := sourceQueryAggregates[key]
	entry.Scope = scope
	entry.Caller = caller
	entry.Mode = mode
	entry.Calls++
	if cacheHit {
		entry.CacheHits++
	}
	if status == "error" {
		entry.Errors++
	}
	if entry.Calls == 1 {
		entry.AvgDurationMS = durationMS
	} else {
		entry.AvgDurationMS = ((entry.AvgDurationMS * float64(entry.Calls-1)) + durationMS) / float64(entry.Calls)
	}
	entry.LastDurationMS = durationMS
	entry.LastStatus = status
	entry.LastWindowStart = windowStart.UTC()
	entry.LastWindowEnd = windowEnd.UTC()
	entry.LastAt = time.Now().UTC()
	sourceQueryAggregates[key] = entry
	sourceQueryAggregatesMu.Unlock()
}

func SourceQueryDiagnosticsSnapshot(limit int) []SourceQueryDiagnosticAggregate {
	if limit <= 0 {
		limit = 10
	}
	if limit > 200 {
		limit = 200
	}

	sourceQueryAggregatesMu.Lock()
	defer sourceQueryAggregatesMu.Unlock()

	out := make([]SourceQueryDiagnosticAggregate, 0, len(sourceQueryAggregates))
	for _, entry := range sourceQueryAggregates {
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Calls == out[j].Calls {
			return out[i].LastAt.After(out[j].LastAt)
		}
		return out[i].Calls > out[j].Calls
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func CeilToMinute(ts time.Time) time.Time {
	ts = ts.UTC()
	floor := ts.Truncate(time.Minute)
	if floor.Equal(ts) {
		return floor
	}
	return floor.Add(time.Minute)
}

func FloorToMinute(ts time.Time) time.Time {
	return ts.UTC().Truncate(time.Minute)
}

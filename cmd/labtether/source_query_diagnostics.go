package main

import (
	"net/http"
	"time"

	"github.com/labtether/labtether/internal/hubapi/shared"
)

// Thin type aliases delegating to internal/hubapi/shared so that callers
// inside cmd/labtether/ keep compiling without a mass rename.

type sourceQueryDiagnosticAggregate = shared.SourceQueryDiagnosticAggregate

func sourceQueryCaller(r *http.Request, fallback string) string {
	return shared.SourceQueryCaller(r, fallback)
}

func normalizeSourceQueryCaller(raw, fallback string) string {
	return shared.NormalizeSourceQueryCaller(raw, fallback)
}

func sourceQueryDiagnosticsEnabled() bool { return shared.SourceQueryDiagnosticsEnabled() }

func logSourceQueryDiagnostic(
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
	shared.LogSourceQueryDiagnostic(scope, caller, mode, groupFiltered, cacheHit, windowStart, windowEnd, limit, sourceCount, startedAt, err)
}

func sourceQueryDiagnosticsSnapshot(limit int) []sourceQueryDiagnosticAggregate {
	return shared.SourceQueryDiagnosticsSnapshot(limit)
}

func ceilToMinute(ts time.Time) time.Time  { return shared.CeilToMinute(ts) }
func floorToMinute(ts time.Time) time.Time { return shared.FloorToMinute(ts) }

package main

import (
	"net/http"

	"github.com/labtether/labtether/internal/hubapi/shared"
)

// Thin aliases delegating to internal/hubapi/shared so that callers
// inside cmd/labtether/ keep compiling without a mass rename.

const (
	streamTraceQueryKey = shared.StreamTraceQueryKey
	streamTraceMaxLen   = shared.StreamTraceMaxLen
)

func browserStreamTraceID(r *http.Request) string { return shared.BrowserStreamTraceID(r) }

func streamTraceLogValue(traceID string) string { return shared.StreamTraceLogValue(traceID) }

func sanitizeBrowserStreamTraceID(raw string) string {
	return shared.SanitizeBrowserStreamTraceID(raw)
}

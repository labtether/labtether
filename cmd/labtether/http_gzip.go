package main

import (
	"net/http"

	"github.com/labtether/labtether/internal/hubapi/shared"
)

// Thin aliases delegating to internal/hubapi/shared so that callers
// inside cmd/labtether/ keep compiling without a mass rename.

func gzipMiddleware(next http.Handler) http.Handler { return shared.GzipMiddleware(next) }

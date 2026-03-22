package main

import "github.com/labtether/labtether/internal/hubapi/shared"

// Thin aliases delegating to internal/hubapi/shared so that callers
// inside cmd/labtether/ keep compiling without a mass rename.

const maxRuntimeCacheEntries = shared.MaxRuntimeCacheEntries

func cloneAnyMap(input map[string]any) map[string]any { return shared.CloneAnyMap(input) }

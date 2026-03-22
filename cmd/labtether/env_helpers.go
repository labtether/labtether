package main

import (
	"time"

	"github.com/labtether/labtether/internal/hubapi/shared"
)

// Thin aliases delegating to internal/hubapi/shared so that callers
// inside cmd/labtether/ keep compiling without a mass rename.

func envOrDefault(key, fallback string) string { return shared.EnvOrDefault(key, fallback) }

func envOrDefaultInt(key string, fallback int) int { return shared.EnvOrDefaultInt(key, fallback) }

func envOrDefaultUint64(key string, fallback uint64) uint64 {
	return shared.EnvOrDefaultUint64(key, fallback)
}

func uint64ToIntClamp(value uint64) int { return shared.Uint64ToIntClamp(value) }

func intToUint64NonNegative(value int) uint64 { return shared.IntToUint64NonNegative(value) }

func envOrDefaultDuration(key string, fallback time.Duration) time.Duration {
	return shared.EnvOrDefaultDuration(key, fallback)
}

func envOrDefaultBool(key string, fallback bool) bool {
	return shared.EnvOrDefaultBool(key, fallback)
}

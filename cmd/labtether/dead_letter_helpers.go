package main

import (
	"time"

	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/persistence"
)

// Thin type aliases delegating to internal/hubapi/shared so that callers
// inside cmd/labtether/ keep compiling without a mass rename.

type deadLetterEventResponse = shared.DeadLetterEventResponse
type deadLetterTopEntry = shared.DeadLetterTopEntry
type deadLetterTrendPoint = shared.DeadLetterTrendPoint
type deadLetterAnalyticsResponse = shared.DeadLetterAnalyticsResponse

func mapLogEventToDeadLetter(event logs.Event) deadLetterEventResponse {
	return shared.MapLogEventToDeadLetter(event)
}

func mapProjectedDeadLetterEvent(event logs.DeadLetterEvent) deadLetterEventResponse {
	return shared.MapProjectedDeadLetterEvent(event)
}

func queryDeadLetterEventResponses(store persistence.LogStore, from, to time.Time, limit int) ([]deadLetterEventResponse, error) {
	return shared.QueryDeadLetterEventResponses(store, from, to, limit)
}

func countDeadLetterEvents(store persistence.LogStore, from, to time.Time) (int, error) {
	return shared.CountDeadLetterEvents(store, from, to)
}

func buildDeadLetterAnalytics(events []deadLetterEventResponse, from, to time.Time, window time.Duration) deadLetterAnalyticsResponse {
	return shared.BuildDeadLetterAnalytics(events, from, to, window)
}

func deadLetterAnalyticsWithTotal(analytics deadLetterAnalyticsResponse, total int, window time.Duration) deadLetterAnalyticsResponse {
	return shared.DeadLetterAnalyticsWithTotal(analytics, total, window)
}

func deadLetterRates(total int, windowHours float64) (float64, float64) {
	return shared.DeadLetterRates(total, windowHours)
}

func chooseDeadLetterBucket(window time.Duration) time.Duration {
	return shared.ChooseDeadLetterBucket(window)
}

func topDeadLetterEntries(counts map[string]int, limit int) []deadLetterTopEntry {
	return shared.TopDeadLetterEntries(counts, limit)
}

func classifyDeadLetterError(message string) string {
	return shared.ClassifyDeadLetterError(message)
}

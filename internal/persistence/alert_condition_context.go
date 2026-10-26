package persistence

import (
	"context"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/synthetic"
	"time"
)

type AlertTelemetryActivityContextStore interface {
	HasTelemetrySamplesContext(context.Context, []string, time.Time, time.Time) (map[string]bool, error)
	AssetsWithMetricSamplesContext(context.Context, []string, string, time.Time, time.Time) (map[string]bool, error)
}

type AlertLogContextStore interface {
	QueryEventsContext(context.Context, logs.QueryRequest) ([]logs.Event, error)
}

type AlertSyntheticContextStore interface {
	GetSyntheticCheckContext(context.Context, string) (synthetic.Check, bool, error)
	ListSyntheticResultsContext(context.Context, string, int) ([]synthetic.Result, error)
}

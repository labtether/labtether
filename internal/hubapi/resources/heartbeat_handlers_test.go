package resources

import (
	"context"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/telemetry"
)

type blockingHeartbeatTelemetryStore struct{}

func (blockingHeartbeatTelemetryStore) AppendSamples(ctx context.Context, _ []telemetry.MetricSample) error {
	<-ctx.Done()
	return ctx.Err()
}

func (blockingHeartbeatTelemetryStore) Snapshot(string, time.Time) (telemetry.Snapshot, error) {
	return telemetry.Snapshot{}, nil
}

func (blockingHeartbeatTelemetryStore) Series(string, time.Time, time.Time, time.Duration) ([]telemetry.Series, error) {
	return nil, nil
}

func TestAppendHeartbeatTelemetrySamplesHonorsTimeout(t *testing.T) {
	started := time.Now()
	err := appendHeartbeatTelemetrySamples(
		blockingHeartbeatTelemetryStore{},
		[]telemetry.MetricSample{{AssetID: "asset-1", Metric: "cpu", Unit: "percent"}},
		10*time.Millisecond,
	)

	if err == nil {
		t.Fatal("expected telemetry append to stop at the timeout")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("telemetry timeout took too long: %s", elapsed)
	}
}

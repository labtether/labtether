package persistence

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/telemetry"
)

func TestValidateMetricSampleBatchEnforcesCountBytesAndContext(t *testing.T) {
	tooMany := make([]telemetry.MetricSample, telemetry.MaxMetricSamplesPerAppend+1)
	if err := validateMetricSampleBatch(context.Background(), tooMany); !errors.Is(err, ErrMetricSampleBatchLimitExceeded) {
		t.Fatalf("count overflow error = %v", err)
	}

	labels := make(map[string]string, 7)
	for i := 0; i < 7; i++ {
		labels[fmt.Sprintf("label-%d", i)] = strings.Repeat("v", telemetry.MaxMetricLabelValueBytes)
	}
	sample := telemetry.MetricSample{AssetID: "asset-1", Metric: "custom_metric", Unit: "count", Value: 1, Labels: labels}
	sampleBytes, err := telemetry.MetricSampleEnvelopeBytes(sample)
	if err != nil {
		t.Fatalf("build bounded sample: %v", err)
	}
	count := telemetry.MaxMetricAppendBytes/sampleBytes + 1
	oversized := make([]telemetry.MetricSample, count)
	for i := range oversized {
		oversized[i] = sample
	}
	if err := validateMetricSampleBatch(context.Background(), oversized); !errors.Is(err, ErrMetricSampleBatchBytesExceeded) {
		t.Fatalf("byte overflow error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := validateMetricSampleBatch(ctx, []telemetry.MetricSample{sample}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled validation error = %v", err)
	}
}

func TestMemoryTelemetryStoreRejectsInvalidEnvelopeWithoutPartialWrite(t *testing.T) {
	store := NewMemoryTelemetryStore()
	err := store.AppendSamples(context.Background(), []telemetry.MetricSample{
		{AssetID: "asset-1", Metric: telemetry.MetricCPUUsedPercent, Unit: "percent", Value: 1},
		{AssetID: "asset-1", Metric: strings.Repeat("x", telemetry.MaxMetricNameBytes+1), Unit: "count", Value: 2},
	})
	if err == nil {
		t.Fatal("invalid metric envelope was accepted")
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	if len(store.samples) != 0 || len(store.hubSamples) != 0 {
		t.Fatalf("invalid batch partially persisted: assets=%v hubs=%v", store.samples, store.hubSamples)
	}
}

func TestMemoryTelemetryStoreRejectsPostgresIncompatibleNULWithoutPartialWrite(t *testing.T) {
	store := NewMemoryTelemetryStore()
	err := store.AppendSamples(context.Background(), []telemetry.MetricSample{
		{AssetID: "asset-1", Metric: telemetry.MetricCPUUsedPercent, Unit: "percent", Value: 1},
		{AssetID: "asset-1", Metric: "custom_metric", Unit: "count", Value: 2, Labels: map[string]string{"source": "bad\x00value"}},
	})
	if err == nil {
		t.Fatal("PostgreSQL-incompatible NUL was accepted")
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	if len(store.samples) != 0 || len(store.hubSamples) != 0 {
		t.Fatalf("NUL-containing batch partially persisted: assets=%v hubs=%v", store.samples, store.hubSamples)
	}
}

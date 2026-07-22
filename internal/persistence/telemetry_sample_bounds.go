package persistence

import (
	"context"
	"fmt"

	"github.com/labtether/labtether/internal/telemetry"
)

func validateMetricSampleBatch(ctx context.Context, samples []telemetry.MetricSample) error {
	if ctx == nil {
		return fmt.Errorf("metric append context is required")
	}
	if len(samples) > telemetry.MaxMetricSamplesPerAppend {
		return ErrMetricSampleBatchLimitExceeded
	}
	totalBytes := 0
	for i, sample := range samples {
		if i%256 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		sampleBytes, err := telemetry.MetricSampleEnvelopeBytes(sample)
		if err != nil {
			return fmt.Errorf("metric sample %d: %w", i, err)
		}
		totalBytes += sampleBytes
		if totalBytes > telemetry.MaxMetricAppendBytes {
			return ErrMetricSampleBatchBytesExceeded
		}
	}
	return nil
}

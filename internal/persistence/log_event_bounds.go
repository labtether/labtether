package persistence

import (
	"fmt"

	"github.com/labtether/labtether/internal/logs"
)

// normalizeLogEventsForInsert validates the complete batch before a memory or
// PostgreSQL write. This preserves all-or-nothing behavior for malformed or
// oversized batches and keeps PostgreSQL safely below its bind ceiling.
func normalizeLogEventsForInsert(events []logs.Event) ([]logs.Event, []string, error) {
	if len(events) > logs.MaxEventsPerBatch {
		return nil, nil, fmt.Errorf("%w: event count %d exceeds %d", logs.ErrEventBatchLimitExceeded, len(events), logs.MaxEventsPerBatch)
	}

	normalized := make([]logs.Event, 0, len(events))
	payloads := make([]string, 0, len(events))
	totalBytes := 2 // JSON array brackets.
	for index, event := range events {
		normalizedEvent, fieldsPayload, eventBytes, err := normalizeLogEventForInsert(event)
		if err != nil {
			return nil, nil, fmt.Errorf("event %d: %w", index, err)
		}
		if index > 0 {
			totalBytes++
		}
		if eventBytes > logs.MaxEventBatchBytes-totalBytes {
			return nil, nil, fmt.Errorf("%w: encoded batch exceeds %d bytes", logs.ErrEventBatchLimitExceeded, logs.MaxEventBatchBytes)
		}
		totalBytes += eventBytes
		normalized = append(normalized, normalizedEvent)
		payloads = append(payloads, fieldsPayload)
	}
	return normalized, payloads, nil
}

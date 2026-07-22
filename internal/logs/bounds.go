package logs

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	// Log event limits are enforced at the shared persistence boundary so every
	// producer (agents, connectors, mobile clients, and internal jobs) receives
	// the same protection in both the memory and PostgreSQL stores.
	MaxEventIDBytes         = 512
	MaxEventAssetIDBytes    = 1024
	MaxEventSourceBytes     = 256
	MaxEventLevelBytes      = 64
	MaxEventMessageBytes    = 64 << 10
	MaxEventFields          = 64
	MaxEventFieldKeyBytes   = 256
	MaxEventFieldValueBytes = 16 << 10
	MaxEventFieldsJSONBytes = 128 << 10
	MaxEventEnvelopeBytes   = 128 << 10

	// Seven PostgreSQL bind parameters are used for each log row. Keeping the
	// batch cap at 1,000 leaves ample headroom below PostgreSQL's 65,535 bind
	// parameter ceiling and also bounds normalization/allocation work.
	MaxEventsPerBatch  = 1000
	MaxEventBatchBytes = 8 << 20
)

var (
	ErrInvalidEventText        = errors.New("invalid log event text")
	ErrEventLimitExceeded      = errors.New("log event limit exceeded")
	ErrEventBatchLimitExceeded = errors.New("log event batch limit exceeded")
)

// EventEnvelopeBytes validates all remotely influenceable event strings and
// returns the exact JSON-encoded event size. Invalid UTF-8 and NUL are rejected
// explicitly because PostgreSQL text cannot store NUL and silently repairing
// invalid text would make memory/PostgreSQL behavior diverge.
func EventEnvelopeBytes(event Event) (int, error) {
	for _, field := range []struct {
		name  string
		value string
		limit int
	}{
		{name: "id", value: event.ID, limit: MaxEventIDBytes},
		{name: "asset_id", value: event.AssetID, limit: MaxEventAssetIDBytes},
		{name: "source", value: event.Source, limit: MaxEventSourceBytes},
		{name: "level", value: event.Level, limit: MaxEventLevelBytes},
		{name: "message", value: event.Message, limit: MaxEventMessageBytes},
	} {
		if err := validateEventText(field.name, field.value, field.limit); err != nil {
			return 0, err
		}
	}

	if len(event.Fields) > MaxEventFields {
		return 0, fmt.Errorf("%w: fields count %d exceeds %d", ErrEventLimitExceeded, len(event.Fields), MaxEventFields)
	}
	for key, value := range event.Fields {
		if err := validateEventText("field key", key, MaxEventFieldKeyBytes); err != nil {
			return 0, err
		}
		if err := validateEventText("field value", value, MaxEventFieldValueBytes); err != nil {
			return 0, err
		}
	}
	fieldsPayload, err := json.Marshal(event.Fields)
	if err != nil {
		return 0, fmt.Errorf("%w: encode fields: %v", ErrInvalidEventText, err)
	}
	if len(fieldsPayload) > MaxEventFieldsJSONBytes {
		return 0, fmt.Errorf("%w: encoded fields bytes %d exceeds %d", ErrEventLimitExceeded, len(fieldsPayload), MaxEventFieldsJSONBytes)
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return 0, fmt.Errorf("%w: encode event: %v", ErrInvalidEventText, err)
	}
	if len(payload) > MaxEventEnvelopeBytes {
		return 0, fmt.Errorf("%w: encoded event bytes %d exceeds %d", ErrEventLimitExceeded, len(payload), MaxEventEnvelopeBytes)
	}
	return len(payload), nil
}

// EventBatchEnvelopeBytes validates a complete batch before any store write and
// returns its exact JSON array size (brackets and commas included).
func EventBatchEnvelopeBytes(events []Event) (int, error) {
	if len(events) > MaxEventsPerBatch {
		return 0, fmt.Errorf("%w: event count %d exceeds %d", ErrEventBatchLimitExceeded, len(events), MaxEventsPerBatch)
	}
	total := 2 // JSON array brackets.
	for index, event := range events {
		size, err := EventEnvelopeBytes(event)
		if err != nil {
			return 0, fmt.Errorf("event %d: %w", index, err)
		}
		if index > 0 {
			total++ // comma
		}
		if size > MaxEventBatchBytes-total {
			return 0, fmt.Errorf("%w: encoded batch exceeds %d bytes", ErrEventBatchLimitExceeded, MaxEventBatchBytes)
		}
		total += size
	}
	return total, nil
}

func validateEventText(name, value string, limit int) error {
	if !utf8.ValidString(value) {
		return fmt.Errorf("%w: %s is not valid UTF-8", ErrInvalidEventText, name)
	}
	if strings.IndexByte(value, 0) >= 0 {
		return fmt.Errorf("%w: %s contains NUL", ErrInvalidEventText, name)
	}
	if len(value) > limit {
		return fmt.Errorf("%w: %s bytes %d exceeds %d", ErrEventLimitExceeded, name, len(value), limit)
	}
	return nil
}

package logs

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestEventEnvelopeTextLimitsExactAndOver(t *testing.T) {
	tests := []struct {
		name  string
		limit int
		set   func(*Event, string)
	}{
		{name: "id", limit: MaxEventIDBytes, set: func(event *Event, value string) { event.ID = value }},
		{name: "asset_id", limit: MaxEventAssetIDBytes, set: func(event *Event, value string) { event.AssetID = value }},
		{name: "source", limit: MaxEventSourceBytes, set: func(event *Event, value string) { event.Source = value }},
		{name: "level", limit: MaxEventLevelBytes, set: func(event *Event, value string) { event.Level = value }},
		{name: "message", limit: MaxEventMessageBytes, set: func(event *Event, value string) { event.Message = value }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			event := Event{Source: "agent", Level: "info", Message: "event"}
			test.set(&event, strings.Repeat("x", test.limit))
			if _, err := EventEnvelopeBytes(event); err != nil {
				t.Fatalf("exact limit rejected: %v", err)
			}
			test.set(&event, strings.Repeat("x", test.limit+1))
			if _, err := EventEnvelopeBytes(event); !errors.Is(err, ErrEventLimitExceeded) {
				t.Fatalf("over limit error = %v, want ErrEventLimitExceeded", err)
			}
		})
	}
}

func TestEventEnvelopeFieldsAndEncodedBounds(t *testing.T) {
	fieldKeyExact := Event{Source: "agent", Level: "info", Message: "event", Fields: map[string]string{strings.Repeat("k", MaxEventFieldKeyBytes): "value"}}
	if _, err := EventEnvelopeBytes(fieldKeyExact); err != nil {
		t.Fatalf("exact field key rejected: %v", err)
	}
	fieldKeyOver := Event{Source: "agent", Level: "info", Message: "event", Fields: map[string]string{strings.Repeat("k", MaxEventFieldKeyBytes+1): "value"}}
	if _, err := EventEnvelopeBytes(fieldKeyOver); !errors.Is(err, ErrEventLimitExceeded) {
		t.Fatalf("over field key error = %v", err)
	}
	fieldValueExact := Event{Source: "agent", Level: "info", Message: "event", Fields: map[string]string{"key": strings.Repeat("v", MaxEventFieldValueBytes)}}
	if _, err := EventEnvelopeBytes(fieldValueExact); err != nil {
		t.Fatalf("exact field value rejected: %v", err)
	}
	fieldValueOver := Event{Source: "agent", Level: "info", Message: "event", Fields: map[string]string{"key": strings.Repeat("v", MaxEventFieldValueBytes+1)}}
	if _, err := EventEnvelopeBytes(fieldValueOver); !errors.Is(err, ErrEventLimitExceeded) {
		t.Fatalf("over field value error = %v", err)
	}

	exact := Event{Source: "agent", Level: "info", Message: "event", Fields: make(map[string]string, MaxEventFields)}
	for index := 0; index < MaxEventFields; index++ {
		exact.Fields[fmt.Sprintf("k%02d", index)] = "v"
	}
	if _, err := EventEnvelopeBytes(exact); err != nil {
		t.Fatalf("exact field count rejected: %v", err)
	}
	exact.Fields["over"] = "v"
	if _, err := EventEnvelopeBytes(exact); !errors.Is(err, ErrEventLimitExceeded) {
		t.Fatalf("over field count error = %v", err)
	}

	for _, event := range []Event{
		{Source: "agent", Level: "info", Message: "bad\x00message"},
		{Source: "agent", Level: "info", Message: string([]byte{0xff})},
		{Source: "agent", Level: "info", Message: "event", Fields: map[string]string{"bad\x00key": "value"}},
		{Source: "agent", Level: "info", Message: "event", Fields: map[string]string{"key": "bad\x00value"}},
	} {
		if _, err := EventEnvelopeBytes(event); !errors.Is(err, ErrInvalidEventText) {
			t.Fatalf("invalid text error = %v, want ErrInvalidEventText", err)
		}
	}

	escaped := Event{Source: "agent", Level: "info", Message: "event", Fields: make(map[string]string)}
	for index := 0; index < 10; index++ {
		escaped.Fields[fmt.Sprintf("escaped_%d", index)] = strings.Repeat("<", MaxEventFieldValueBytes)
	}
	if _, err := EventEnvelopeBytes(escaped); !errors.Is(err, ErrEventLimitExceeded) {
		t.Fatalf("encoded field expansion error = %v, want ErrEventLimitExceeded", err)
	}
}

func TestEventEnvelopeTotalBytesExactAndOver(t *testing.T) {
	event := Event{Source: "agent", Level: "info", Message: strings.Repeat("x", MaxEventMessageBytes), Fields: map[string]string{}}
	for index := 0; ; index++ {
		current, err := EventEnvelopeBytes(event)
		if err != nil {
			t.Fatalf("build exact envelope: %v", err)
		}
		if current == MaxEventEnvelopeBytes {
			break
		}
		key := fmt.Sprintf("k%02d", index)
		event.Fields[key] = ""
		withField, err := EventEnvelopeBytes(event)
		if err != nil {
			t.Fatalf("add field for exact envelope: %v", err)
		}
		remaining := MaxEventEnvelopeBytes - withField
		if remaining < 0 {
			t.Fatal("empty field unexpectedly exceeded envelope")
		}
		if remaining > MaxEventFieldValueBytes {
			remaining = MaxEventFieldValueBytes
		}
		event.Fields[key] = strings.Repeat("x", remaining)
	}
	if size, err := EventEnvelopeBytes(event); err != nil || size != MaxEventEnvelopeBytes {
		t.Fatalf("exact envelope size=%d error=%v", size, err)
	}
	for key, value := range event.Fields {
		if len(value) < MaxEventFieldValueBytes {
			event.Fields[key] = value + "x"
			break
		}
	}
	if _, err := EventEnvelopeBytes(event); !errors.Is(err, ErrEventLimitExceeded) {
		t.Fatalf("over envelope error = %v, want ErrEventLimitExceeded", err)
	}
}

func TestEventBatchCountAndBytesExactAndOver(t *testing.T) {
	countExact := make([]Event, MaxEventsPerBatch)
	if _, err := EventBatchEnvelopeBytes(countExact); err != nil {
		t.Fatalf("exact event count rejected: %v", err)
	}
	if _, err := EventBatchEnvelopeBytes(append(countExact, Event{})); !errors.Is(err, ErrEventBatchLimitExceeded) {
		t.Fatalf("over event count error = %v", err)
	}

	byteExact := make([]Event, 128)
	for index := range byteExact {
		byteExact[index] = Event{Source: "agent", Level: "info"}
	}
	base, err := EventBatchEnvelopeBytes(byteExact)
	if err != nil {
		t.Fatalf("measure base batch: %v", err)
	}
	remaining := MaxEventBatchBytes - base
	for index := range byteExact {
		added := min(remaining, MaxEventMessageBytes)
		byteExact[index].Message = strings.Repeat("x", added)
		remaining -= added
		if remaining == 0 {
			break
		}
	}
	if remaining != 0 {
		t.Fatalf("could not construct exact batch; %d bytes remain", remaining)
	}
	if size, err := EventBatchEnvelopeBytes(byteExact); err != nil || size != MaxEventBatchBytes {
		t.Fatalf("exact batch size=%d error=%v", size, err)
	}
	for index := range byteExact {
		if len(byteExact[index].Message) < MaxEventMessageBytes {
			byteExact[index].Message += "x"
			break
		}
	}
	if _, err := EventBatchEnvelopeBytes(byteExact); !errors.Is(err, ErrEventBatchLimitExceeded) {
		t.Fatalf("over batch bytes error = %v, want ErrEventBatchLimitExceeded", err)
	}
}

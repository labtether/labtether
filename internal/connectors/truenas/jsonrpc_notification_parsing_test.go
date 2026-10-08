package truenas

import (
	"encoding/json"
	"testing"
)

func TestParseSubscriptionEventIgnoresRPCResponses(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	_, ok := parseSubscriptionEvent(map[string]any{
		"jsonrpc": "2.0",
		"id":      99,
		"result":  true,
	}, "alert.list", "sub-1")
	if ok {
		t.Fatalf("expected parseSubscriptionEvent to ignore rpc result envelope")
	}
}

func TestParseSubscriptionIDAndIdentifierHelpers(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	if got := parseSubscriptionID(nil); got != "" {
		t.Fatalf("parseSubscriptionID(nil) = %q, want empty", got)
	}
	if got := parseSubscriptionID(json.RawMessage("{")); got != "" {
		t.Fatalf("parseSubscriptionID(invalid) = %q, want empty", got)
	}
	if got := parseSubscriptionID(json.RawMessage(`123`)); got != "123" {
		t.Fatalf("parseSubscriptionID(number) = %q, want 123", got)
	}

	tests := []struct {
		name  string
		value any
		want  string
	}{
		{name: "nil", value: nil, want: ""},
		{name: "string", value: " abc ", want: "abc"},
		{name: "float64", value: float64(5), want: "5"},
		{name: "float64 huge", value: 1e100, want: ""},
		{name: "float64 fractional", value: 5.5, want: ""},
		{name: "float32", value: float32(7), want: "7"},
		{name: "int", value: 9, want: "9"},
		{name: "int64", value: int64(11), want: "11"},
		{name: "uint64", value: uint64(13), want: "13"},
		{name: "json number", value: json.Number("15"), want: "15"},
		{name: "stringer", value: testIdentifierStringer("value"), want: "value"},
		{name: "fallback fmt", value: true, want: "true"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := anyToIdentifier(tt.value); got != tt.want {
				t.Fatalf("anyToIdentifier(%v) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestParseNotificationParamsShapes(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	collection, messageType, fields, subID := parseNotificationParams(nil)
	if collection != "" || messageType != "" || fields != nil || subID != "" {
		t.Fatalf("unexpected decode for nil params")
	}

	collection, messageType, fields, subID = parseNotificationParams([]any{
		"sub-1",
		"added",
		"alert.list",
		map[string]any{"uuid": "alert-5"},
	})
	if collection != "alert.list" || messageType != "added" || subID != "sub-1" {
		t.Fatalf("unexpected 4-arg decode: collection=%q msg=%q sub=%q", collection, messageType, subID)
	}
	if got := anyToIdentifier(fields["uuid"]); got != "alert-5" {
		t.Fatalf("unexpected fields uuid %q", got)
	}

	collection, messageType, fields, subID = parseNotificationParams([]any{
		"alert.list",
		"changed",
		map[string]any{"uuid": "alert-6"},
	})
	if collection != "map[uuid:alert-6]" || messageType != "changed" || subID != "alert.list" {
		t.Fatalf("unexpected alternate shape decode: collection=%q msg=%q sub=%q", collection, messageType, subID)
	}
	if got := anyToIdentifier(fields["uuid"]); got != "alert-6" {
		t.Fatalf("unexpected alternate fields uuid %q", got)
	}
}

func TestParseSubscriptionEventAdditionalBranches(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	t.Run("ignore rpc error envelope", func(t *testing.T) {
		_, ok := parseSubscriptionEvent(map[string]any{
			"id":    1,
			"error": map[string]any{"code": -32000},
		}, "alert.list", "sub-1")
		if ok {
			t.Fatalf("expected rpc error envelope to be ignored")
		}
	})

	t.Run("fallback by subscription id when collection mismatched", func(t *testing.T) {
		event, ok := parseSubscriptionEvent(map[string]any{
			"id":         "sub-1",
			"collection": "pool.query",
			"msg":        "changed",
			"fields": map[string]any{
				"uuid": "pool-1",
			},
		}, "alert.list", "sub-1")
		if !ok {
			t.Fatalf("expected event to pass via subscription id fallback")
		}
		if event.Collection != "pool.query" {
			t.Fatalf("event collection = %q, want pool.query", event.Collection)
		}
	})

	t.Run("filter mismatched collection and subscription id", func(t *testing.T) {
		_, ok := parseSubscriptionEvent(map[string]any{
			"id":         "sub-other",
			"collection": "pool.query",
			"msg":        "changed",
			"fields":     map[string]any{"uuid": "pool-2"},
		}, "alert.list", "sub-1")
		if ok {
			t.Fatalf("expected mismatched event to be filtered")
		}
	})

	t.Run("notification params without fields are still surfaced with collection context", func(t *testing.T) {
		event, ok := parseSubscriptionEvent(map[string]any{
			"method": "collection_update",
			"params": []any{"sub-1", "changed", "alert.list"},
		}, "alert.list", "sub-1")
		if !ok {
			t.Fatalf("expected event to be surfaced")
		}
		if event.Collection != "alert.list" {
			t.Fatalf("event collection = %q, want alert.list", event.Collection)
		}
	})
}

func TestParseNotificationParamsAdditionalBranches(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	collection, messageType, fields, subscriptionID := parseNotificationParams([]any{})
	if collection != "" || messageType != "" || fields != nil || subscriptionID != "" {
		t.Fatalf("expected empty decode for empty params slice")
	}

	collection, messageType, fields, subscriptionID = parseNotificationParams([]any{
		"sub-only",
		"changed",
		"alert.list",
		"not-a-map",
	})
	if collection != "alert.list" || messageType != "changed" || subscriptionID != "sub-only" || fields != nil {
		t.Fatalf("unexpected decode for non-map fields payload: collection=%q msg=%q sub=%q fields=%#v", collection, messageType, subscriptionID, fields)
	}
}

func TestParseSubscriptionEventFieldsOverrideAndDefaults(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	event, ok := parseSubscriptionEvent(map[string]any{
		"method": "collection_update",
		"params": []any{"sub-1", "", "alert.list", map[string]any{"uuid": "from-params"}},
		"fields": map[string]any{"uuid": "from-fields"},
	}, "alert.list", "sub-1")
	if !ok {
		t.Fatalf("expected event to be parsed")
	}
	if event.MessageType != "event" {
		t.Fatalf("expected default message type event, got %q", event.MessageType)
	}
	if got := anyToIdentifier(event.Fields["uuid"]); got != "from-fields" {
		t.Fatalf("expected explicit fields payload to override params payload, got %q", got)
	}
}

func TestParseSubscriptionEventCollectionFallbackAndEmptyPayloadRejection(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	event, ok := parseSubscriptionEvent(map[string]any{
		"msg": "changed",
	}, "alert.list", "sub-1")
	if !ok {
		t.Fatalf("expected event with expected collection fallback")
	}
	if event.Collection != "alert.list" {
		t.Fatalf("event collection = %q, want alert.list", event.Collection)
	}

	if _, ok := parseSubscriptionEvent(map[string]any{
		"msg": "changed",
	}, "", ""); ok {
		t.Fatalf("expected empty collection+fields event to be rejected")
	}
}

func TestParseNotificationParamsLenOneCollectionFallback(t *testing.T) {
	allowInsecureTransportForTrueNASTests(t)
	collection, messageType, fields, subscriptionID := parseNotificationParams([]any{"alert.list"})
	if collection != "alert.list" {
		t.Fatalf("collection = %q, want alert.list", collection)
	}
	if messageType != "" || fields != nil {
		t.Fatalf("unexpected message/fields for len1 params: msg=%q fields=%#v", messageType, fields)
	}
	if subscriptionID != "alert.list" {
		t.Fatalf("subscriptionID = %q, want alert.list", subscriptionID)
	}
}

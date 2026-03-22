package persistence

import (
	"encoding/json"
	"testing"
)

func TestLabelsToJSONArg(t *testing.T) {
	t.Run("nil map returns nil nil", func(t *testing.T) {
		got, err := labelsToJSONArg(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})

	t.Run("empty map returns nil nil", func(t *testing.T) {
		got, err := labelsToJSONArg(map[string]string{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})

	t.Run("populated map returns JSON string", func(t *testing.T) {
		labels := map[string]string{
			"region": "us-east-1",
			"env":    "prod",
		}
		got, err := labelsToJSONArg(labels)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		s, ok := got.(string)
		if !ok {
			t.Fatalf("expected string, got %T", got)
		}

		// Verify the JSON is valid and round-trips to the same map.
		var parsed map[string]string
		if err := json.Unmarshal([]byte(s), &parsed); err != nil {
			t.Fatalf("result is not valid JSON: %v", err)
		}
		for k, want := range labels {
			if got := parsed[k]; got != want {
				t.Errorf("key %q: got %q, want %q", k, got, want)
			}
		}
		if len(parsed) != len(labels) {
			t.Errorf("parsed map has %d keys, want %d", len(parsed), len(labels))
		}
	})

	t.Run("single key map returns valid JSON", func(t *testing.T) {
		labels := map[string]string{"host": "web-01"}
		got, err := labelsToJSONArg(labels)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		s, ok := got.(string)
		if !ok {
			t.Fatalf("expected string, got %T", got)
		}
		var parsed map[string]string
		if err := json.Unmarshal([]byte(s), &parsed); err != nil {
			t.Fatalf("result is not valid JSON: %v", err)
		}
		if parsed["host"] != "web-01" {
			t.Errorf("expected host=web-01, got %q", parsed["host"])
		}
	})
}

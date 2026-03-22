package main

import (
	"testing"
	"time"

	"github.com/labtether/labtether/internal/logs"
)

// Tests cover the local aliases defined in dead_letter_helpers.go.
// Functions requiring a persistence.LogStore (queryDeadLetterEventResponses,
// countDeadLetterEvents) are omitted because they require database plumbing.
// The remaining helpers are pure or near-pure functions that can be validated
// in isolation.

// ---------------------------------------------------------------------------
// classifyDeadLetterError
// ---------------------------------------------------------------------------

func TestDeadLetterClassifyDeadLetterError(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"empty message", "", "unknown"},
		{"timeout keyword", "request timeout exceeded", "timeout"},
		{"timed out phrase", "operation timed out", "timeout"},
		{"deadline exceeded", "context deadline exceeded", "timeout"},
		{"unauthorized", "401 unauthorized", "auth"},
		{"forbidden", "403 forbidden", "auth"},
		{"auth keyword", "invalid auth token", "auth"},
		{"decode keyword", "failed to decode response", "decode"},
		{"unmarshal keyword", "json unmarshal error", "decode"},
		{"parse keyword", "unable to parse payload", "decode"},
		{"invalid json phrase", "invalid json received", "decode"},
		{"dial keyword", "dial tcp: connection refused", "network"},
		{"connection keyword", "connection reset by peer", "network"},
		{"network keyword", "network unreachable", "network"},
		{"refused keyword", "connect: connection refused", "network"},
		{"unreachable keyword", "host is unreachable", "network"},
		{"not found phrase", "resource not found", "not_found"},
		{"permission keyword", "permission denied", "permission"},
		{"invalid keyword", "invalid request body", "validation"},
		{"bad request phrase", "bad request: missing field", "validation"},
		{"validation keyword", "validation failed", "validation"},
		{"unknown error", "something completely unexpected happened", "other"},
		{"mixed case timeout", "Request Timeout", "timeout"},
		{"whitespace padded", "  timeout  ", "timeout"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyDeadLetterError(tc.input); got != tc.want {
				t.Fatalf("classifyDeadLetterError(%q): expected %q, got %q", tc.input, tc.want, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// chooseDeadLetterBucket
// ---------------------------------------------------------------------------

func TestDeadLetterChooseDeadLetterBucket(t *testing.T) {
	cases := []struct {
		name   string
		window time.Duration
		want   time.Duration
	}{
		{"1 hour window → 5 min bucket", time.Hour, 5 * time.Minute},
		{"2 hour window → 5 min bucket", 2 * time.Hour, 5 * time.Minute},
		{"3 hour window → 1 hour bucket", 3 * time.Hour, time.Hour},
		{"12 hour window → 1 hour bucket", 12 * time.Hour, time.Hour},
		{"24 hour window → 1 hour bucket", 24 * time.Hour, time.Hour},
		{"2 day window → 6 hour bucket", 2 * 24 * time.Hour, 6 * time.Hour},
		{"7 day window → 6 hour bucket", 7 * 24 * time.Hour, 6 * time.Hour},
		{"8 day window → 24 hour bucket", 8 * 24 * time.Hour, 24 * time.Hour},
		{"30 day window → 24 hour bucket", 30 * 24 * time.Hour, 24 * time.Hour},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := chooseDeadLetterBucket(tc.window); got != tc.want {
				t.Fatalf("chooseDeadLetterBucket(%v): expected %v, got %v", tc.window, tc.want, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// deadLetterRates
// ---------------------------------------------------------------------------

func TestDeadLetterRates(t *testing.T) {
	t.Run("zero window returns 0,0", func(t *testing.T) {
		rph, rpd := deadLetterRates(100, 0)
		if rph != 0 || rpd != 0 {
			t.Fatalf("expected (0, 0), got (%v, %v)", rph, rpd)
		}
	})

	t.Run("negative window returns 0,0", func(t *testing.T) {
		rph, rpd := deadLetterRates(100, -5)
		if rph != 0 || rpd != 0 {
			t.Fatalf("expected (0, 0), got (%v, %v)", rph, rpd)
		}
	})

	t.Run("24 hour window with 48 events", func(t *testing.T) {
		rph, rpd := deadLetterRates(48, 24)
		if rph != 2 {
			t.Fatalf("expected rate_per_hour=2, got %v", rph)
		}
		if rpd != 48 {
			t.Fatalf("expected rate_per_day=48, got %v", rpd)
		}
	})

	t.Run("1 hour window with 10 events", func(t *testing.T) {
		rph, rpd := deadLetterRates(10, 1)
		if rph != 10 {
			t.Fatalf("expected rate_per_hour=10, got %v", rph)
		}
		// 1 hour / 24 = ~0.0417 day; rate_per_day = 10/0.0417 ≈ 240
		if rpd != 240 {
			t.Fatalf("expected rate_per_day=240, got %v", rpd)
		}
	})

	t.Run("zero total always returns 0 rates", func(t *testing.T) {
		rph, rpd := deadLetterRates(0, 24)
		if rph != 0 || rpd != 0 {
			t.Fatalf("expected (0, 0), got (%v, %v)", rph, rpd)
		}
	})
}

// ---------------------------------------------------------------------------
// topDeadLetterEntries
// ---------------------------------------------------------------------------

func TestDeadLetterTopDeadLetterEntries(t *testing.T) {
	t.Run("empty map returns empty slice", func(t *testing.T) {
		got := topDeadLetterEntries(map[string]int{}, 5)
		if len(got) != 0 {
			t.Fatalf("expected empty slice, got %v", got)
		}
	})

	t.Run("nil map returns empty slice", func(t *testing.T) {
		got := topDeadLetterEntries(nil, 5)
		if len(got) != 0 {
			t.Fatalf("expected empty slice for nil map, got %v", got)
		}
	})

	t.Run("results sorted by count descending", func(t *testing.T) {
		counts := map[string]int{"a": 3, "b": 10, "c": 1}
		got := topDeadLetterEntries(counts, 10)
		if len(got) != 3 {
			t.Fatalf("expected 3 entries, got %d", len(got))
		}
		if got[0].Key != "b" || got[0].Count != 10 {
			t.Fatalf("expected first entry (b,10), got (%s,%d)", got[0].Key, got[0].Count)
		}
		if got[1].Key != "a" || got[1].Count != 3 {
			t.Fatalf("expected second entry (a,3), got (%s,%d)", got[1].Key, got[1].Count)
		}
		if got[2].Key != "c" || got[2].Count != 1 {
			t.Fatalf("expected third entry (c,1), got (%s,%d)", got[2].Key, got[2].Count)
		}
	})

	t.Run("limit applied", func(t *testing.T) {
		counts := map[string]int{"a": 5, "b": 10, "c": 1, "d": 8, "e": 3}
		got := topDeadLetterEntries(counts, 3)
		if len(got) != 3 {
			t.Fatalf("expected 3 entries after limit, got %d", len(got))
		}
		// Top 3 by count: b(10), d(8), a(5)
		if got[0].Key != "b" {
			t.Fatalf("expected top entry b, got %s", got[0].Key)
		}
	})

	t.Run("ties broken alphabetically by key", func(t *testing.T) {
		counts := map[string]int{"z": 5, "a": 5, "m": 5}
		got := topDeadLetterEntries(counts, 10)
		if len(got) != 3 {
			t.Fatalf("expected 3 entries, got %d", len(got))
		}
		if got[0].Key != "a" || got[1].Key != "m" || got[2].Key != "z" {
			t.Fatalf("expected alphabetical tie-break order [a,m,z], got [%s,%s,%s]",
				got[0].Key, got[1].Key, got[2].Key)
		}
	})

	t.Run("zero limit returns all entries", func(t *testing.T) {
		counts := map[string]int{"x": 1, "y": 2}
		got := topDeadLetterEntries(counts, 0)
		if len(got) != 2 {
			t.Fatalf("expected 2 entries for limit=0, got %d", len(got))
		}
	})
}

// ---------------------------------------------------------------------------
// mapLogEventToDeadLetter
// ---------------------------------------------------------------------------

func TestDeadLetterMapLogEventToDeadLetter(t *testing.T) {
	ts := time.Date(2025, 3, 1, 12, 0, 0, 0, time.UTC)

	t.Run("full fields populated", func(t *testing.T) {
		event := logs.Event{
			ID:        "fallback-id",
			Message:   "fallback error message",
			Timestamp: ts,
			Fields: map[string]string{
				"event_id":    "evt-123",
				"component":   "broker",
				"subject":     "metrics.collect",
				"deliveries":  "3",
				"error":       "connection refused",
				"payload_b64": "dGVzdA==",
			},
		}

		got := mapLogEventToDeadLetter(event)

		if got.ID != "evt-123" {
			t.Errorf("expected ID evt-123, got %q", got.ID)
		}
		if got.Component != "broker" {
			t.Errorf("expected component broker, got %q", got.Component)
		}
		if got.Subject != "metrics.collect" {
			t.Errorf("expected subject metrics.collect, got %q", got.Subject)
		}
		if got.Deliveries != 3 {
			t.Errorf("expected deliveries 3, got %d", got.Deliveries)
		}
		if got.Error != "connection refused" {
			t.Errorf("expected error 'connection refused', got %q", got.Error)
		}
		if got.PayloadB64 != "dGVzdA==" {
			t.Errorf("expected payload_b64 'dGVzdA==', got %q", got.PayloadB64)
		}
		if !got.CreatedAt.Equal(ts) {
			t.Errorf("expected CreatedAt %v, got %v", ts, got.CreatedAt)
		}
	})

	t.Run("missing event_id falls back to event.ID", func(t *testing.T) {
		event := logs.Event{
			ID:        "event-fallback",
			Timestamp: ts,
			Fields:    map[string]string{},
		}
		got := mapLogEventToDeadLetter(event)
		if got.ID != "event-fallback" {
			t.Fatalf("expected fallback ID event-fallback, got %q", got.ID)
		}
	})

	t.Run("missing error field falls back to event.Message", func(t *testing.T) {
		event := logs.Event{
			ID:        "evt",
			Message:   "event level message",
			Timestamp: ts,
			Fields:    map[string]string{},
		}
		got := mapLogEventToDeadLetter(event)
		if got.Error != "event level message" {
			t.Fatalf("expected error from Message, got %q", got.Error)
		}
	})

	t.Run("nil fields handled gracefully", func(t *testing.T) {
		event := logs.Event{
			ID:        "e1",
			Timestamp: ts,
			Fields:    nil,
		}
		got := mapLogEventToDeadLetter(event)
		if got.Deliveries != 0 {
			t.Fatalf("expected 0 deliveries for nil fields, got %d", got.Deliveries)
		}
	})

	t.Run("timestamp converted to UTC", func(t *testing.T) {
		loc, _ := time.LoadLocation("America/New_York")
		localTS := time.Date(2025, 3, 1, 8, 0, 0, 0, loc)
		event := logs.Event{
			ID:        "e2",
			Timestamp: localTS,
			Fields:    map[string]string{},
		}
		got := mapLogEventToDeadLetter(event)
		if got.CreatedAt.Location() != time.UTC {
			t.Fatalf("expected UTC location, got %v", got.CreatedAt.Location())
		}
	})
}

// ---------------------------------------------------------------------------
// mapProjectedDeadLetterEvent
// ---------------------------------------------------------------------------

func TestDeadLetterMapProjectedDeadLetterEvent(t *testing.T) {
	ts := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	event := logs.DeadLetterEvent{
		ID:         "  dl-001  ",
		Component:  " queue ",
		Subject:    " alerts.send ",
		Deliveries: 7,
		Error:      " timeout exceeded ",
		PayloadB64: " abc123 ",
		CreatedAt:  ts,
	}

	got := mapProjectedDeadLetterEvent(event)

	if got.ID != "dl-001" {
		t.Errorf("expected trimmed ID dl-001, got %q", got.ID)
	}
	if got.Component != "queue" {
		t.Errorf("expected trimmed component queue, got %q", got.Component)
	}
	if got.Subject != "alerts.send" {
		t.Errorf("expected trimmed subject alerts.send, got %q", got.Subject)
	}
	if got.Deliveries != 7 {
		t.Errorf("expected deliveries 7, got %d", got.Deliveries)
	}
	if got.Error != "timeout exceeded" {
		t.Errorf("expected trimmed error, got %q", got.Error)
	}
	if got.PayloadB64 != "abc123" {
		t.Errorf("expected trimmed PayloadB64 abc123, got %q", got.PayloadB64)
	}
	if !got.CreatedAt.Equal(ts) {
		t.Errorf("expected CreatedAt %v, got %v", ts, got.CreatedAt)
	}
}

// ---------------------------------------------------------------------------
// deadLetterAnalyticsWithTotal
// ---------------------------------------------------------------------------

func TestDeadLetterAnalyticsWithTotal(t *testing.T) {
	base := deadLetterAnalyticsResponse{
		Window:      "24h0m0s",
		Bucket:      "1h0m0s",
		Total:       10,
		RatePerHour: 0.5,
		RatePerDay:  12,
	}

	t.Run("updates total and recomputes rates", func(t *testing.T) {
		window := 24 * time.Hour
		got := deadLetterAnalyticsWithTotal(base, 48, window)
		if got.Total != 48 {
			t.Errorf("expected total 48, got %d", got.Total)
		}
		if got.RatePerHour != 2 {
			t.Errorf("expected rate_per_hour 2, got %v", got.RatePerHour)
		}
		if got.RatePerDay != 48 {
			t.Errorf("expected rate_per_day 48, got %v", got.RatePerDay)
		}
	})

	t.Run("negative total clamped to zero", func(t *testing.T) {
		got := deadLetterAnalyticsWithTotal(base, -1, 24*time.Hour)
		if got.Total != 0 {
			t.Errorf("expected total 0 for negative input, got %d", got.Total)
		}
	})
}

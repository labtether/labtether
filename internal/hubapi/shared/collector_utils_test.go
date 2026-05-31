package shared

import (
	"math"
	"testing"
	"time"
)

func TestNumericHelpersRejectNonFiniteValues(t *testing.T) {
	if got := AnyToFloat64("NaN"); got != 0 {
		t.Fatalf("AnyToFloat64(NaN) = %v, want 0", got)
	}
	if got := AnyToFloat64(math.Inf(1)); got != 0 {
		t.Fatalf("AnyToFloat64(+Inf) = %v, want 0", got)
	}
	if got, ok := ParseAnyInt64(math.Inf(1)); ok {
		t.Fatalf("ParseAnyInt64(+Inf) = %v, true; want false", got)
	}
	if got, ok := ParseAnyInt64(1e100); ok {
		t.Fatalf("ParseAnyInt64(1e100) = %v, true; want false", got)
	}
	if got, ok := ParseAnyInt64(42.5); !ok || got != 42 {
		t.Fatalf("ParseAnyInt64(42.5) = %v, %v; want 42, true", got, ok)
	}
	if got, ok := ParseAnyBoolLoose(math.NaN()); ok {
		t.Fatalf("ParseAnyBoolLoose(NaN) = %v, true; want false", got)
	}
	if got, ok := ParseAnyTimestamp("Inf"); ok {
		t.Fatalf("ParseAnyTimestamp(Inf) = %s, true; want false", got)
	}
}

func TestCollectorAnyTimeRejectsInfiniteUnixTimestamp(t *testing.T) {
	before := time.Now().Add(-time.Second)
	got := CollectorAnyTime(math.Inf(1))
	after := time.Now().Add(time.Second)
	if got.Before(before) || got.After(after) {
		t.Fatalf("CollectorAnyTime(+Inf) = %s, want current fallback between %s and %s", got, before, after)
	}
}

func TestTimestampHelpersRejectOutOfRangeUnixTimestamp(t *testing.T) {
	before := time.Now().Add(-time.Second)
	got := CollectorAnyTime(1e100)
	after := time.Now().Add(time.Second)
	if got.Before(before) || got.After(after) {
		t.Fatalf("CollectorAnyTime(1e100) = %s, want current fallback between %s and %s", got, before, after)
	}

	if got, ok := ParseAnyTimestamp(1e100); ok {
		t.Fatalf("ParseAnyTimestamp(1e100) = %s, true; want false", got)
	}
}

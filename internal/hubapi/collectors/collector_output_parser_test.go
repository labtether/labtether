package collectors

import (
	"encoding/json"
	"math"
	"testing"
	"time"
)

func TestParseCollectorOutputSkipsNonFiniteJSONValues(t *testing.T) {
	samples, err := ParseCollectorOutput(`{"cpu":"NaN","memory":"Inf","disk":"-Inf","temp":"42.5"}`, "json", "asset-1")
	if err != nil {
		t.Fatalf("ParseCollectorOutput() error = %v", err)
	}

	if len(samples) != 1 {
		t.Fatalf("ParseCollectorOutput() returned %d samples, want 1: %#v", len(samples), samples)
	}
	if samples[0].Metric != MetricTempCelsius || samples[0].Value != 42.5 {
		t.Fatalf("remaining sample = %#v, want temperature 42.5", samples[0])
	}
	if _, err := json.Marshal(samples); err != nil {
		t.Fatalf("marshal finite samples: %v", err)
	}
}

func TestParseCollectorOutputSkipsNonFiniteArrayValues(t *testing.T) {
	samples, err := ParseCollectorOutput(`[{"metric":"cpu","value":"NaN"},{"metric":"disk","value":71.25}]`, "json", "asset-1")
	if err != nil {
		t.Fatalf("ParseCollectorOutput() error = %v", err)
	}

	if len(samples) != 1 {
		t.Fatalf("ParseCollectorOutput() returned %d samples, want 1: %#v", len(samples), samples)
	}
	if samples[0].Metric != MetricDiskPercent || samples[0].Value != 71.25 {
		t.Fatalf("remaining sample = %#v, want disk 71.25", samples[0])
	}
}

func TestParseCompatConfidenceRejectsNonFiniteValues(t *testing.T) {
	for _, raw := range []string{"NaN", "Inf", "-Inf"} {
		if got := parseCompatConfidence(raw); got != 0 {
			t.Fatalf("parseCompatConfidence(%q) = %v, want 0", raw, got)
		}
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

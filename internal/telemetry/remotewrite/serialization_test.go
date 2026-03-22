package remotewrite

import (
	"math"
	"testing"

	"github.com/golang/snappy"
	"google.golang.org/protobuf/encoding/protowire"
)

// ----------------------------------------------------------------------------
// protobuf parsing helpers
//
// These decode the hand-rolled WriteRequest/TimeSeries/Label/Sample messages
// produced by SerializeWriteRequest so we can make structural assertions
// without importing the full github.com/prometheus/prometheus module.
//
// Schema (field numbers from prompb/remote.proto):
//   WriteRequest  { timeseries: repeated TimeSeries  = 1 }
//   TimeSeries    { labels: repeated Label = 1; samples: repeated Sample = 2 }
//   Label         { name: string = 1; value: string = 2 }
//   Sample        { value: double = 1; timestamp: int64 = 2 }
// ----------------------------------------------------------------------------

type parsedLabel struct {
	name  string
	value string
}

type parsedSample struct {
	value     float64
	timestamp int64
}

type parsedTimeSeries struct {
	labels  []parsedLabel
	samples []parsedSample
}

type parsedWriteRequest struct {
	timeSeries []parsedTimeSeries
}

func parseWriteRequest(b []byte) (parsedWriteRequest, error) {
	var wr parsedWriteRequest
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return wr, protowire.ParseError(n)
		}
		b = b[n:]

		if num == 1 && typ == protowire.BytesType {
			// timeseries field
			val, n := protowire.ConsumeBytes(b)
			if n < 0 {
				return wr, protowire.ParseError(n)
			}
			b = b[n:]
			ts, err := parseTimeSeries(val)
			if err != nil {
				return wr, err
			}
			wr.timeSeries = append(wr.timeSeries, ts)
		} else {
			// Unknown field: skip.
			n := protowire.ConsumeFieldValue(num, typ, b)
			if n < 0 {
				return wr, protowire.ParseError(n)
			}
			b = b[n:]
		}
	}
	return wr, nil
}

func parseTimeSeries(b []byte) (parsedTimeSeries, error) {
	var ts parsedTimeSeries
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return ts, protowire.ParseError(n)
		}
		b = b[n:]

		switch {
		case num == 1 && typ == protowire.BytesType:
			// labels field
			val, n := protowire.ConsumeBytes(b)
			if n < 0 {
				return ts, protowire.ParseError(n)
			}
			b = b[n:]
			lbl, err := parseLabel(val)
			if err != nil {
				return ts, err
			}
			ts.labels = append(ts.labels, lbl)
		case num == 2 && typ == protowire.BytesType:
			// samples field
			val, n := protowire.ConsumeBytes(b)
			if n < 0 {
				return ts, protowire.ParseError(n)
			}
			b = b[n:]
			s, err := parseSample(val)
			if err != nil {
				return ts, err
			}
			ts.samples = append(ts.samples, s)
		default:
			n := protowire.ConsumeFieldValue(num, typ, b)
			if n < 0 {
				return ts, protowire.ParseError(n)
			}
			b = b[n:]
		}
	}
	return ts, nil
}

func parseLabel(b []byte) (parsedLabel, error) {
	var lbl parsedLabel
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return lbl, protowire.ParseError(n)
		}
		b = b[n:]

		switch {
		case num == 1 && typ == protowire.BytesType:
			s, n := protowire.ConsumeString(b)
			if n < 0 {
				return lbl, protowire.ParseError(n)
			}
			b = b[n:]
			lbl.name = s
		case num == 2 && typ == protowire.BytesType:
			s, n := protowire.ConsumeString(b)
			if n < 0 {
				return lbl, protowire.ParseError(n)
			}
			b = b[n:]
			lbl.value = s
		default:
			n := protowire.ConsumeFieldValue(num, typ, b)
			if n < 0 {
				return lbl, protowire.ParseError(n)
			}
			b = b[n:]
		}
	}
	return lbl, nil
}

func parseSample(b []byte) (parsedSample, error) {
	var s parsedSample
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return s, protowire.ParseError(n)
		}
		b = b[n:]

		switch {
		case num == 1 && typ == protowire.Fixed64Type:
			bits, n := protowire.ConsumeFixed64(b)
			if n < 0 {
				return s, protowire.ParseError(n)
			}
			b = b[n:]
			s.value = math.Float64frombits(bits)
		case num == 2 && typ == protowire.VarintType:
			v, n := protowire.ConsumeVarint(b)
			if n < 0 {
				return s, protowire.ParseError(n)
			}
			b = b[n:]
			s.timestamp = int64(v)
		default:
			n := protowire.ConsumeFieldValue(num, typ, b)
			if n < 0 {
				return s, protowire.ParseError(n)
			}
			b = b[n:]
		}
	}
	return s, nil
}

// labelMap converts a slice of parsedLabel to a map for easy lookup.
func labelMap(labels []parsedLabel) map[string]string {
	m := make(map[string]string, len(labels))
	for _, l := range labels {
		m[l.name] = l.value
	}
	return m
}

// ----------------------------------------------------------------------------
// Round-trip tests
// ----------------------------------------------------------------------------

// TestSerializeWriteRequestRoundTrip verifies that SerializeWriteRequest
// produces snappy-compressed protobuf that can be decoded back to recover
// the exact labels, values, and timestamps supplied as input.
func TestSerializeWriteRequestRoundTrip(t *testing.T) {
	const ts1 int64 = 1700000000000
	const ts2 int64 = 1700000001000

	input := []SampleWithLabels{
		{
			Labels:    map[string]string{"__name__": "labtether_cpu_used_percent", "asset_id": "asset-1", "asset_type": "linux"},
			Value:     42.5,
			Timestamp: ts1,
		},
		{
			Labels:    map[string]string{"__name__": "labtether_memory_used_percent", "asset_id": "asset-1", "asset_type": "linux"},
			Value:     60.0,
			Timestamp: ts1,
		},
		{
			Labels:    map[string]string{"__name__": "labtether_cpu_used_percent", "asset_id": "asset-2", "asset_type": "linux"},
			Value:     15.0,
			Timestamp: ts2,
		},
	}

	body, err := SerializeWriteRequest(input)
	if err != nil {
		t.Fatalf("SerializeWriteRequest: %v", err)
	}
	if len(body) == 0 {
		t.Fatal("expected non-empty serialized body")
	}

	// Decompress the snappy payload.
	decoded, err := snappy.Decode(nil, body)
	if err != nil {
		t.Fatalf("snappy.Decode: %v", err)
	}
	if len(decoded) == 0 {
		t.Fatal("decoded protobuf is empty")
	}

	// Parse the WriteRequest from the raw protobuf bytes.
	wr, err := parseWriteRequest(decoded)
	if err != nil {
		t.Fatalf("parseWriteRequest: %v", err)
	}

	// Three samples with distinct label fingerprints → three TimeSeries.
	if len(wr.timeSeries) != 3 {
		t.Fatalf("expected 3 TimeSeries, got %d", len(wr.timeSeries))
	}

	// Build a lookup: __name__ + asset_id → parsed time series for assertions.
	type key struct{ name, assetID string }
	byKey := make(map[key]parsedTimeSeries, len(wr.timeSeries))
	for _, ts := range wr.timeSeries {
		lm := labelMap(ts.labels)
		k := key{name: lm["__name__"], assetID: lm["asset_id"]}
		byKey[k] = ts
	}

	// Verify cpu metric for asset-1.
	cpuA1, ok := byKey[key{"labtether_cpu_used_percent", "asset-1"}]
	if !ok {
		t.Fatal("missing TimeSeries for labtether_cpu_used_percent asset-1")
	}
	if len(cpuA1.samples) != 1 {
		t.Fatalf("expected 1 sample for cpu/asset-1, got %d", len(cpuA1.samples))
	}
	if cpuA1.samples[0].value != 42.5 {
		t.Errorf("cpu/asset-1 value: want 42.5, got %v", cpuA1.samples[0].value)
	}
	if cpuA1.samples[0].timestamp != ts1 {
		t.Errorf("cpu/asset-1 timestamp: want %d, got %d", ts1, cpuA1.samples[0].timestamp)
	}

	// Verify __name__ is present in labels.
	lm1 := labelMap(cpuA1.labels)
	if lm1["__name__"] != "labtether_cpu_used_percent" {
		t.Errorf("__name__ label: want labtether_cpu_used_percent, got %q", lm1["__name__"])
	}
	if lm1["asset_type"] != "linux" {
		t.Errorf("asset_type label: want linux, got %q", lm1["asset_type"])
	}

	// Verify memory metric for asset-1.
	memA1, ok := byKey[key{"labtether_memory_used_percent", "asset-1"}]
	if !ok {
		t.Fatal("missing TimeSeries for labtether_memory_used_percent asset-1")
	}
	if len(memA1.samples) != 1 || memA1.samples[0].value != 60.0 {
		t.Errorf("memory/asset-1: want value=60.0 got %v", memA1.samples)
	}

	// Verify cpu metric for asset-2.
	cpuA2, ok := byKey[key{"labtether_cpu_used_percent", "asset-2"}]
	if !ok {
		t.Fatal("missing TimeSeries for labtether_cpu_used_percent asset-2")
	}
	if len(cpuA2.samples) != 1 || cpuA2.samples[0].value != 15.0 {
		t.Errorf("cpu/asset-2: want value=15.0 got %v", cpuA2.samples)
	}
	if cpuA2.samples[0].timestamp != ts2 {
		t.Errorf("cpu/asset-2 timestamp: want %d, got %d", ts2, cpuA2.samples[0].timestamp)
	}
}

// TestSerializeGroupsSameLabels verifies that two samples sharing an identical
// label set are encoded in a single TimeSeries with two Sample entries.
func TestSerializeGroupsSameLabels(t *testing.T) {
	now := int64(1700000000000)
	labels := map[string]string{"__name__": "labtether_cpu_used_percent", "host": "server1"}

	input := []SampleWithLabels{
		{Labels: labels, Value: 10.0, Timestamp: now},
		{Labels: labels, Value: 20.0, Timestamp: now + 15000},
	}

	body, err := SerializeWriteRequest(input)
	if err != nil {
		t.Fatalf("SerializeWriteRequest: %v", err)
	}

	decoded, err := snappy.Decode(nil, body)
	if err != nil {
		t.Fatalf("snappy.Decode: %v", err)
	}

	wr, err := parseWriteRequest(decoded)
	if err != nil {
		t.Fatalf("parseWriteRequest: %v", err)
	}

	// Both samples have identical labels → exactly one TimeSeries.
	if len(wr.timeSeries) != 1 {
		t.Fatalf("expected 1 TimeSeries for identical labels, got %d", len(wr.timeSeries))
	}

	ts := wr.timeSeries[0]
	if len(ts.samples) != 2 {
		t.Fatalf("expected 2 samples in merged TimeSeries, got %d", len(ts.samples))
	}

	// Values are preserved in insertion order.
	if ts.samples[0].value != 10.0 {
		t.Errorf("sample[0].value: want 10.0, got %v", ts.samples[0].value)
	}
	if ts.samples[1].value != 20.0 {
		t.Errorf("sample[1].value: want 20.0, got %v", ts.samples[1].value)
	}
	if ts.samples[0].timestamp != now {
		t.Errorf("sample[0].timestamp: want %d, got %d", now, ts.samples[0].timestamp)
	}
	if ts.samples[1].timestamp != now+15000 {
		t.Errorf("sample[1].timestamp: want %d, got %d", now+15000, ts.samples[1].timestamp)
	}
}

// TestSerializeNameLabelSortsFirst verifies that __name__ appears as the first
// label in each encoded TimeSeries per Prometheus convention.
func TestSerializeNameLabelSortsFirst(t *testing.T) {
	input := []SampleWithLabels{
		{
			Labels: map[string]string{
				"zone":     "us-east",
				"__name__": "disk_free",
				"host":     "db1",
				"asset_id": "asset-db1",
			},
			Value:     512000.0,
			Timestamp: 1700000000000,
		},
	}

	body, err := SerializeWriteRequest(input)
	if err != nil {
		t.Fatalf("SerializeWriteRequest: %v", err)
	}
	decoded, err := snappy.Decode(nil, body)
	if err != nil {
		t.Fatalf("snappy.Decode: %v", err)
	}
	wr, err := parseWriteRequest(decoded)
	if err != nil {
		t.Fatalf("parseWriteRequest: %v", err)
	}

	if len(wr.timeSeries) != 1 {
		t.Fatalf("expected 1 TimeSeries, got %d", len(wr.timeSeries))
	}
	labels := wr.timeSeries[0].labels
	if len(labels) == 0 {
		t.Fatal("expected labels, got none")
	}
	if labels[0].name != "__name__" {
		t.Errorf("first label should be __name__, got %q", labels[0].name)
	}
	if labels[0].value != "disk_free" {
		t.Errorf("__name__ label value: want disk_free, got %q", labels[0].value)
	}
}

// TestSerializePreservesSpecialFloat verifies that special float64 values
// (NaN, +Inf, -Inf) survive the encode/decode round-trip.
func TestSerializePreservesSpecialFloat(t *testing.T) {
	cases := []struct {
		name  string
		value float64
	}{
		{"nan", math.NaN()},
		{"pos_inf", math.Inf(1)},
		{"neg_inf", math.Inf(-1)},
		{"zero", 0.0},
		{"negative", -1.5},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := []SampleWithLabels{
				{
					Labels:    map[string]string{"__name__": "test_metric"},
					Value:     tc.value,
					Timestamp: 1700000000000,
				},
			}

			body, err := SerializeWriteRequest(input)
			if err != nil {
				t.Fatalf("SerializeWriteRequest: %v", err)
			}
			decoded, err := snappy.Decode(nil, body)
			if err != nil {
				t.Fatalf("snappy.Decode: %v", err)
			}
			wr, err := parseWriteRequest(decoded)
			if err != nil {
				t.Fatalf("parseWriteRequest: %v", err)
			}
			if len(wr.timeSeries) != 1 || len(wr.timeSeries[0].samples) != 1 {
				t.Fatalf("unexpected structure: %v", wr)
			}
			got := wr.timeSeries[0].samples[0].value
			if math.IsNaN(tc.value) {
				if !math.IsNaN(got) {
					t.Errorf("value: want NaN, got %v", got)
				}
			} else if got != tc.value {
				t.Errorf("value: want %v, got %v", tc.value, got)
			}
		})
	}
}

// TestSerializeEmptyRoundTrip verifies that nil/empty input returns nil, nil.
func TestSerializeEmptyRoundTrip(t *testing.T) {
	body, err := SerializeWriteRequest(nil)
	if err != nil {
		t.Fatalf("nil input: unexpected error: %v", err)
	}
	if body != nil {
		t.Fatalf("nil input: expected nil body, got %d bytes", len(body))
	}

	body, err = SerializeWriteRequest([]SampleWithLabels{})
	if err != nil {
		t.Fatalf("empty slice: unexpected error: %v", err)
	}
	if body != nil {
		t.Fatalf("empty slice: expected nil body, got %d bytes", len(body))
	}
}

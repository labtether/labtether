package remotewrite

import (
	"math"
	"strings"
	"testing"

	"github.com/golang/snappy"
)

func TestSerializeWideLabelsAndTimestampRoundTrip(t *testing.T) {
	name := strings.Repeat("x", MaxLabelNameBytes)
	value := strings.Repeat("界", MaxLabelValueBytes/3)
	body, err := SerializeWriteRequest([]SampleWithLabels{{
		Labels: map[string]string{"__name__": "wide_labels", name: value},
		Value:  math.Inf(1), Timestamp: math.MaxInt64,
	}})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := snappy.Decode(nil, body)
	if err != nil {
		t.Fatal(err)
	}
	request, err := parseWriteRequest(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(request.timeSeries) != 1 {
		t.Fatalf("got %d series", len(request.timeSeries))
	}
	series := request.timeSeries[0]
	if len(series.labels) != 2 || series.labels[1].name != name || series.labels[1].value != value {
		t.Fatal("wide UTF-8 label did not survive wire encoding")
	}
	if len(series.samples) != 1 || series.samples[0].timestamp != math.MaxInt64 || !math.IsInf(series.samples[0].value, 1) {
		t.Fatalf("sample changed during wire encoding: %+v", series.samples)
	}
}

package collectors

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/telemetry"
)

func TestBoundedCollectorOutputAppliesOneConcurrentStreamBudget(t *testing.T) {
	output := newBoundedCollectorOutput(1024)
	writers := []interface{ Write([]byte) (int, error) }{
		output.stdoutWriter().(boundedCollectorStream),
		output.stderrWriter().(boundedCollectorStream),
	}

	var wg sync.WaitGroup
	for _, writer := range writers {
		writer := writer
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 32 {
				_, _ = writer.Write([]byte(strings.Repeat("x", 64)))
			}
		}()
	}
	wg.Wait()

	got, overflow := output.snapshot(true)
	if !overflow {
		t.Fatal("combined stdout/stderr writes did not report overflow")
	}
	if len(got) > 1024 {
		t.Fatalf("bounded output length = %d, want <= 1024", len(got))
	}
}

func TestReadRemoteCollectorOutputRejectsOverflowInsteadOfTruncating(t *testing.T) {
	got, err := readRemoteCollectorOutput(strings.NewReader("1234"), 4)
	if err != nil || got != "1234" {
		t.Fatalf("exact-limit read = (%q, %v), want (1234, nil)", got, err)
	}

	got, err = readRemoteCollectorOutput(strings.NewReader("12345"), 4)
	if !errors.Is(err, errRemoteCollectorOutputLimit) {
		t.Fatalf("overflow error = %v, want %v", err, errRemoteCollectorOutputLimit)
	}
	if got != "" {
		t.Fatalf("overflow returned partial body %q", got)
	}
}

func TestAppendCollectorOutputLogUsesSeparateSmallUTF8SafeBudget(t *testing.T) {
	store := persistence.NewMemoryLogStore()
	deps := &Deps{LogStore: store}
	output := strings.Repeat("x", maxPersistedCollectorLogBytes) + "€" + strings.Repeat("y", 1024)

	deps.appendCollectorOutputLog("asset-1", "hub-collector", "info", output)
	events, err := store.QueryEvents(logs.QueryRequest{
		AssetID: "asset-1",
		From:    time.Now().Add(-time.Minute),
		To:      time.Now().Add(time.Minute),
		Limit:   1,
	})
	if err != nil {
		t.Fatalf("QueryEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("persisted event count = %d, want 1", len(events))
	}
	message := events[0].Message
	if len(message) > maxPersistedCollectorLogBytes {
		t.Fatalf("persisted message bytes = %d, want <= %d", len(message), maxPersistedCollectorLogBytes)
	}
	if !strings.HasSuffix(message, collectorLogOutputTruncatedMarker) {
		t.Fatalf("persisted message missing truncation marker: %q", message)
	}
	if !utf8.ValidString(message) {
		t.Fatal("persisted message is not valid UTF-8")
	}
}

func TestCollectorOutputForLogNormalizesInvalidUTF8(t *testing.T) {
	got := collectorOutputForLog("ok\xffbad")
	if !utf8.ValidString(got) {
		t.Fatalf("collectorOutputForLog returned invalid UTF-8: %q", got)
	}
	if !strings.Contains(got, "\uFFFD") {
		t.Fatalf("collectorOutputForLog did not replace invalid UTF-8: %q", got)
	}
}

func TestParseCollectorOutputAssignsStableUnits(t *testing.T) {
	samples, err := ParseCollectorOutput(
		`{"cpu":12.5,"temperature":41,"network_rx":99,"custom.gauge":7}`,
		"json",
		"asset-1",
	)
	if err != nil {
		t.Fatalf("ParseCollectorOutput: %v", err)
	}
	units := make(map[string]string, len(samples))
	for _, sample := range samples {
		units[sample.Metric] = sample.Unit
	}
	want := map[string]string{
		MetricCPUPercent:       "percent",
		MetricTempCelsius:      "celsius",
		MetricNetRXBytesPerSec: "bytes_per_sec",
		"custom.gauge":         "value",
	}
	for metric, unit := range want {
		if units[metric] != unit {
			t.Errorf("unit for %q = %q, want %q", metric, units[metric], unit)
		}
	}
}

type recordingCollectorTelemetryStore struct {
	samples []telemetry.MetricSample
	err     error
}

func (s *recordingCollectorTelemetryStore) AppendSamples(ctx context.Context, samples []telemetry.MetricSample) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.samples = append(s.samples, samples...)
	return s.err
}

func (*recordingCollectorTelemetryStore) Snapshot(string, time.Time) (telemetry.Snapshot, error) {
	return telemetry.Snapshot{}, nil
}

func (*recordingCollectorTelemetryStore) Series(string, time.Time, time.Time, time.Duration) ([]telemetry.Series, error) {
	return nil, nil
}

func TestIngestCollectorTelemetryPersistsUnitsAndReturnsStoreFailure(t *testing.T) {
	store := &recordingCollectorTelemetryStore{}
	deps := &Deps{TelemetryStore: store}
	collector := hubcollector.Collector{
		AssetID: "asset-1",
		Config:  map[string]any{"response_format": "json"},
	}
	if err := deps.ingestCollectorTelemetry(context.Background(), collector, `{"cpu":17,"custom_metric":9}`); err != nil {
		t.Fatalf("ingestCollectorTelemetry: %v", err)
	}
	if len(store.samples) != 2 {
		t.Fatalf("persisted sample count = %d, want 2", len(store.samples))
	}
	for _, sample := range store.samples {
		if sample.Unit == "" {
			t.Fatalf("persisted sample %q has an empty unit", sample.Metric)
		}
	}

	store.err = errors.New("store unavailable")
	if err := deps.ingestCollectorTelemetry(context.Background(), collector, `{"cpu":18}`); !errors.Is(err, store.err) {
		t.Fatalf("store failure = %v, want %v", err, store.err)
	}

	store.err = nil
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := deps.ingestCollectorTelemetry(canceled, collector, `{"cpu":19}`); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled ingest error = %v, want %v", err, context.Canceled)
	}
}

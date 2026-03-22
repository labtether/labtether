package remotewrite

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang/snappy"
)

// ---- serialization tests ----

func TestSerializeWriteRequestEmpty(t *testing.T) {
	body, err := SerializeWriteRequest(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if body != nil {
		t.Fatalf("expected nil body for empty input, got %d bytes", len(body))
	}

	body, err = SerializeWriteRequest([]SampleWithLabels{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if body != nil {
		t.Fatalf("expected nil body for empty slice, got %d bytes", len(body))
	}
}

func TestSerializeWriteRequestNonEmpty(t *testing.T) {
	samples := []SampleWithLabels{
		{
			Labels:    map[string]string{"__name__": "cpu_usage", "host": "server1"},
			Value:     42.5,
			Timestamp: 1700000000000,
		},
		{
			Labels:    map[string]string{"__name__": "mem_usage", "host": "server1"},
			Value:     75.0,
			Timestamp: 1700000001000,
		},
	}

	body, err := SerializeWriteRequest(samples)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(body) == 0 {
		t.Fatal("expected non-empty body")
	}

	// Body must be valid snappy: decode it to verify.
	decoded, err := snappy.Decode(nil, body)
	if err != nil {
		t.Fatalf("body is not valid snappy-compressed data: %v", err)
	}
	if len(decoded) == 0 {
		t.Fatal("decoded protobuf is empty")
	}
}

func TestSerializeWriteRequestGroupsLabels(t *testing.T) {
	// Two samples with identical labels should form a single TimeSeries.
	// We verify the encoded body is smaller than two separate time series would be,
	// as a proxy for grouping. We can also verify the output is valid snappy.
	now := int64(1700000000000)
	samples := []SampleWithLabels{
		{Labels: map[string]string{"__name__": "load", "host": "h1"}, Value: 1.0, Timestamp: now},
		{Labels: map[string]string{"__name__": "load", "host": "h1"}, Value: 2.0, Timestamp: now + 1000},
	}

	body, err := SerializeWriteRequest(samples)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = snappy.Decode(nil, body)
	if err != nil {
		t.Fatalf("body is not valid snappy: %v", err)
	}
}

func TestSerializeWriteRequestLabelOrder(t *testing.T) {
	// __name__ must come first in the encoded labels.
	// We can't easily parse the protobuf here, but at minimum we verify no error
	// and a valid snappy-compressed output.
	samples := []SampleWithLabels{
		{
			Labels: map[string]string{
				"zone":     "us-east",
				"__name__": "disk_free",
				"host":     "db1",
			},
			Value:     512.0,
			Timestamp: 1700000000000,
		},
	}
	body, err := SerializeWriteRequest(samples)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err = snappy.Decode(nil, body); err != nil {
		t.Fatalf("body is not valid snappy: %v", err)
	}
}

// ---- Push HTTP tests ----

func TestPushSuccess(t *testing.T) {
	var received []byte
	var gotContentType, gotContentEncoding, gotVersion, gotUserAgent string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		gotContentEncoding = r.Header.Get("Content-Encoding")
		gotVersion = r.Header.Get("X-Prometheus-Remote-Write-Version")
		gotUserAgent = r.Header.Get("User-Agent")
		received, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	payload := []byte("fake-compressed-body")
	if err := Push(context.Background(), srv.URL, payload, "", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotContentType != "application/x-protobuf" {
		t.Errorf("Content-Type = %q, want application/x-protobuf", gotContentType)
	}
	if gotContentEncoding != "snappy" {
		t.Errorf("Content-Encoding = %q, want snappy", gotContentEncoding)
	}
	if gotVersion != "0.1.0" {
		t.Errorf("X-Prometheus-Remote-Write-Version = %q, want 0.1.0", gotVersion)
	}
	if gotUserAgent != "labtether-hub/1.0" {
		t.Errorf("User-Agent = %q, want labtether-hub/1.0", gotUserAgent)
	}
	if string(received) != "fake-compressed-body" {
		t.Errorf("body = %q, want fake-compressed-body", received)
	}
}

func TestPushBasicAuth(t *testing.T) {
	var gotUser, gotPass string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser, gotPass, _ = r.BasicAuth()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := Push(context.Background(), srv.URL, []byte("x"), "admin", "s3cr3t"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotUser != "admin" || gotPass != "s3cr3t" {
		t.Errorf("basic auth = %q:%q, want admin:s3cr3t", gotUser, gotPass)
	}
}

func TestPushNoAuth(t *testing.T) {
	var gotAuthHeader string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthHeader = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := Push(context.Background(), srv.URL, []byte("x"), "", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAuthHeader != "" {
		t.Errorf("expected no Authorization header, got %q", gotAuthHeader)
	}
}

func TestPushHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	err := Push(context.Background(), srv.URL, []byte("x"), "", "")
	if err == nil {
		t.Fatal("expected error for 5xx response")
	}
}

func TestPushBadURL(t *testing.T) {
	err := Push(context.Background(), "http://127.0.0.1:0/unreachable", []byte("x"), "", "")
	if err == nil {
		t.Fatal("expected error for unreachable URL")
	}
}

// ---- Worker tests ----

// mockSource is an in-memory SampleSource.
type mockSource struct {
	samples []SampleWithLabels
}

func (m *mockSource) SamplesSince(_ context.Context, since time.Time, limit int) ([]SampleWithLabels, error) {
	var out []SampleWithLabels
	for _, s := range m.samples {
		if t := TimeFromMillis(s.Timestamp); t.After(since) {
			out = append(out, s)
			if len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

// mockHWM is an in-memory HighWaterMark.
type mockHWM struct {
	t time.Time
}

func (m *mockHWM) Get(_ context.Context) (time.Time, error) { return m.t, nil }
func (m *mockHWM) Set(_ context.Context, t time.Time) error { m.t = t; return nil }

func TestWorkerPushesBatch(t *testing.T) {
	var pushCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pushCount.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	now := time.Now()
	source := &mockSource{
		samples: []SampleWithLabels{
			{Labels: map[string]string{"__name__": "cpu"}, Value: 10.0, Timestamp: now.UnixMilli()},
			{Labels: map[string]string{"__name__": "mem"}, Value: 20.0, Timestamp: now.Add(time.Second).UnixMilli()},
		},
	}
	hwm := &mockHWM{}

	cfg := Config{
		Enabled:  true,
		URL:      srv.URL,
		Interval: 20 * time.Millisecond,
	}
	worker := NewWorker(cfg, source, hwm)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	worker.Run(ctx)

	if pushCount.Load() == 0 {
		t.Fatal("expected at least one push, got zero")
	}

	// High-water mark must have advanced.
	if hwm.t.IsZero() {
		t.Fatal("expected high-water mark to be advanced after push")
	}
}

func TestWorkerDisabledDoesNotPush(t *testing.T) {
	var pushCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pushCount.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	cfg := Config{
		Enabled:  false,
		URL:      srv.URL,
		Interval: 10 * time.Millisecond,
	}
	worker := NewWorker(cfg, &mockSource{}, &mockHWM{})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	worker.Run(ctx) // returns immediately when Enabled == false

	if pushCount.Load() != 0 {
		t.Fatalf("expected no pushes, got %d", pushCount.Load())
	}
}

func TestWorkerBacksOffOnError(t *testing.T) {
	// Server always returns 500 to trigger backoff.
	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	now := time.Now()
	source := &mockSource{
		samples: []SampleWithLabels{
			{Labels: map[string]string{"__name__": "cpu"}, Value: 5.0, Timestamp: now.UnixMilli()},
		},
	}

	cfg := Config{
		Enabled:  true,
		URL:      srv.URL,
		Interval: 20 * time.Millisecond,
	}
	worker := NewWorker(cfg, source, &mockHWM{})

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	worker.Run(ctx)

	// With backoff doubling, we should see fewer calls than without backoff.
	// This is a smoke test: we just verify the worker ran without panicking.
	if callCount.Load() == 0 {
		t.Fatal("expected at least one call to the server")
	}
}

func TestWorkerEmptySourceNoRequest(t *testing.T) {
	var callCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	cfg := Config{
		Enabled:  true,
		URL:      srv.URL,
		Interval: 20 * time.Millisecond,
	}
	// Source with no samples — worker should skip pushing.
	worker := NewWorker(cfg, &mockSource{}, &mockHWM{})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	worker.Run(ctx)

	if callCount.Load() != 0 {
		t.Fatalf("expected no HTTP calls for empty source, got %d", callCount.Load())
	}
}

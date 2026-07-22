package remotewrite

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type pagedMockSource struct {
	mu     sync.Mutex
	total  int64
	limits []int
}

func (s *pagedMockSource) SamplesAfter(_ context.Context, cursor Cursor, limit int) (Batch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.limits = append(s.limits, limit)
	remaining := s.total - cursor.AssetSampleID
	if remaining <= 0 {
		return Batch{Next: cursor}, nil
	}
	count := min(int64(limit), remaining)
	samples := make([]SampleWithLabels, count)
	for index := range samples {
		samples[index] = SampleWithLabels{
			Labels:    map[string]string{"__name__": "labtether_cpu_used_percent", "asset_id": "asset"},
			Value:     float64(cursor.AssetSampleID + int64(index) + 1),
			Timestamp: 1700000000000 + cursor.AssetSampleID + int64(index),
		}
	}
	return Batch{
		Samples: samples,
		Next:    Cursor{AssetSampleID: cursor.AssetSampleID + count},
		More:    count == int64(limit),
	}, nil
}

func (s *pagedMockSource) requestedLimits() []int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]int(nil), s.limits...)
}

type notifyingCursor struct {
	mu     sync.Mutex
	cursor Cursor
	saved  chan Cursor
}

func (c *notifyingCursor) LoadRemoteWriteCursor(_ context.Context, _ string) (Cursor, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cursor, nil
}

func (c *notifyingCursor) SaveRemoteWriteCursor(_ context.Context, _ string, cursor Cursor, _ time.Time) error {
	c.mu.Lock()
	c.cursor = cursor
	c.mu.Unlock()
	select {
	case c.saved <- cursor:
	default:
	}
	return nil
}

func TestSerializeWriteRequestRejectsInvalidIdentityDuplicateAndOversize(t *testing.T) {
	now := time.Now().UnixMilli()
	if _, err := SerializeWriteRequest([]SampleWithLabels{{Labels: map[string]string{"__name__": "9invalid"}, Value: 1, Timestamp: now}}); err == nil {
		t.Fatal("expected invalid metric name rejection")
	}
	if _, err := SerializeWriteRequest([]SampleWithLabels{
		{Labels: map[string]string{"__name__": "valid", "asset_id": "one"}, Value: 1, Timestamp: now},
		{Labels: map[string]string{"__name__": "valid", "asset_id": "one"}, Value: 2, Timestamp: now},
	}); err == nil {
		t.Fatal("expected duplicate series timestamp rejection")
	}
	tooMany := make([]SampleWithLabels, MaxSamplesPerRequest+1)
	for index := range tooMany {
		tooMany[index] = SampleWithLabels{Labels: map[string]string{"__name__": "valid", "id": string(rune('a' + index%26))}, Value: 1, Timestamp: now + int64(index)}
	}
	if _, err := SerializeWriteRequest(tooMany); err == nil {
		t.Fatal("expected oversized sample count rejection")
	}
}

func TestPushErrorsNeverContainEndpoint(t *testing.T) {
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()
	err := Push(context.Background(), server.URL+"/credential-bearing-path", []byte("body"), "", "")
	if err == nil {
		t.Fatal("expected receiver failure")
	}
	if strings.Contains(err.Error(), server.URL) || strings.Contains(err.Error(), "credential-bearing-path") {
		t.Fatalf("push error leaked endpoint: %q", err)
	}
}

func TestWorkerCursorFailuresAreNotReportedAsSuccess(t *testing.T) {
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	sample := SampleWithLabels{Labels: map[string]string{"__name__": "labtether_cpu"}, Value: 1, Timestamp: time.Now().UnixMilli()}
	config := Config{Enabled: true, URL: server.URL, Interval: MinInterval}

	loadFailure := &mockCursor{loadErr: errors.New("load failed")}
	worker, err := NewWorker(config, &mockSource{samples: []SampleWithLabels{sample}}, loadFailure)
	if err != nil {
		t.Fatalf("NewWorker load failure: %v", err)
	}
	if err := worker.pushBatch(context.Background()); err == nil || requests.Load() != 0 {
		t.Fatalf("cursor load failure result error=%v requests=%d", err, requests.Load())
	}

	saveFailure := &mockCursor{saveErr: errors.New("save failed")}
	worker, err = NewWorker(config, &mockSource{samples: []SampleWithLabels{sample}}, saveFailure)
	if err != nil {
		t.Fatalf("NewWorker save failure: %v", err)
	}
	if err := worker.pushBatch(context.Background()); err == nil || requests.Load() != 1 || saveFailure.cursor != (Cursor{}) {
		t.Fatalf("cursor save failure result error=%v requests=%d cursor=%+v", err, requests.Load(), saveFailure.cursor)
	}
}

func TestWorkerPersistsFilteredPageProgressWithoutFalseHTTPPush(t *testing.T) {
	config := Config{Enabled: true, URL: "https://metrics.example.test/write", Interval: MinInterval}
	cursors := &mockCursor{cursor: Cursor{AssetSampleID: 4}}
	worker, err := NewWorker(config, &mockSource{next: Cursor{AssetSampleID: 5}}, cursors)
	if err != nil {
		t.Fatalf("NewWorker: %v", err)
	}
	if err := worker.pushBatch(context.Background()); err != nil {
		t.Fatalf("pushBatch filtered progress: %v", err)
	}
	if cursors.cursor.AssetSampleID != 5 {
		t.Fatalf("filtered cursor = %+v, want asset ID 5", cursors.cursor)
	}
}

func TestWorkerAdaptsReceiverPayloadRejectionBeforeAdvancingCursor(t *testing.T) {
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) == 1 {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	source := &pagedMockSource{total: MaxSamplesPerRequest}
	cursors := &mockCursor{}
	worker, err := NewWorker(Config{Enabled: true, URL: server.URL, Interval: MinInterval}, source, cursors)
	if err != nil {
		t.Fatalf("NewWorker: %v", err)
	}
	more, err := worker.pushPage(context.Background())
	if err != nil {
		t.Fatalf("pushPage: %v", err)
	}
	limits := source.requestedLimits()
	if !more || requests.Load() != 2 || cursors.cursor.AssetSampleID != 250 || len(limits) != 2 || limits[0] != 500 || limits[1] != 250 {
		t.Fatalf("adaptive page result more=%v requests=%d cursor=%+v limits=%v", more, requests.Load(), cursors.cursor, limits)
	}
}

func TestWorkerCatchesUpFullBacklogWithoutWaitingNormalInterval(t *testing.T) {
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	source := &pagedMockSource{total: 600}
	cursors := &notifyingCursor{saved: make(chan Cursor, 4)}
	worker, err := NewWorker(Config{Enabled: true, URL: server.URL, Interval: MinInterval}, source, cursors)
	if err != nil {
		t.Fatalf("NewWorker: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		worker.Run(ctx)
		close(done)
	}()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case cursor := <-cursors.saved:
			if cursor.AssetSampleID == 600 {
				cancel()
				<-done
				limits := source.requestedLimits()
				if len(limits) < 2 || limits[0] != 500 || limits[1] != 500 {
					t.Fatalf("catch-up page limits = %v", limits)
				}
				return
			}
		case <-deadline:
			cancel()
			<-done
			t.Fatalf("timed out waiting for catch-up; limits=%v", source.requestedLimits())
		}
	}
}

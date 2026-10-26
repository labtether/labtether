package remotewrite

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang/snappy"
)

type rejectedPageSource struct{ timestamps []int64 }

func (s rejectedPageSource) SamplesAfter(_ context.Context, cursor Cursor, _ int) (Batch, error) {
	if cursor.AssetSampleID >= int64(len(s.timestamps)) {
		return Batch{Next: cursor}, nil
	}
	return Batch{Samples: []SampleWithLabels{{Labels: map[string]string{"__name__": "temperature"},
		Value: 1, Timestamp: s.timestamps[cursor.AssetSampleID]}},
		Next: Cursor{AssetSampleID: cursor.AssetSampleID + 1}, More: cursor.AssetSampleID+1 < int64(len(s.timestamps))}, nil
}

// The HTTP owner tests model definite rollback and a successful commit whose
// reply was lost. The PostgreSQL fixture separately proves the real transaction.
type rejectionCursor struct {
	mockCursor
	pages            []RejectedPage
	calls, delivered int
	failure          string
}

func (s *rejectionCursor) SaveRemoteWriteCursor(ctx context.Context, fp string, c Cursor, at time.Time) error {
	s.delivered++
	return s.mockCursor.SaveRemoteWriteCursor(ctx, fp, c, at)
}

func (s *rejectionCursor) RejectRemoteWritePage(_ context.Context, _ string, p RejectedPage) error {
	s.calls++
	if s.calls == 1 && s.failure == "rollback" {
		return ErrRejectedStoreFull
	}
	if len(s.pages) == 0 {
		p.Payload = bytes.Clone(p.Payload)
		s.pages = append(s.pages, p)
		s.cursor = p.Next
	}
	if s.calls == 1 && s.failure == "lost commit reply" {
		return errors.New("commit response unavailable")
	}
	return nil
}

func TestWorkerRetainsPermanentRejectionAndContinues(t *testing.T) {
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")
	for _, failure := range []string{"", "rollback", "lost commit reply"} {
		t.Run(failure, func(t *testing.T) {
			var requests atomic.Int32
			var receiverMu sync.Mutex
			var lastTimestamp int64
			var rejectedBody []byte
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				body, err := io.ReadAll(io.LimitReader(r.Body, MaxRequestBodyBytes+1))
				if err != nil {
					t.Error(err)
					w.WriteHeader(500)
					return
				}
				decoded, err := snappy.Decode(nil, body)
				if err != nil {
					t.Error(err)
					w.WriteHeader(500)
					return
				}
				message, err := parseWriteRequest(decoded)
				if err != nil || len(message.timeSeries) != 1 || len(message.timeSeries[0].samples) != 1 {
					t.Errorf("unexpected actual wire page: %+v/%v", message, err)
					w.WriteHeader(500)
					return
				}
				ts := message.timeSeries[0].samples[0].timestamp
				receiverMu.Lock()
				defer receiverMu.Unlock()
				if ts < lastTimestamp {
					rejectedBody = bytes.Clone(body)
					w.WriteHeader(http.StatusBadRequest)
					_, _ = w.Write([]byte("private receiver detail must not be stored"))
					return
				}
				lastTimestamp = ts
				w.WriteHeader(http.StatusNoContent)
			}))
			defer server.Close()
			config := Config{Enabled: true, URL: server.URL, Interval: MinInterval}
			source := rejectedPageSource{[]int64{2000, 1000, 3000}}
			store := &rejectionCursor{failure: failure}
			worker, err := NewWorker(config, source, store)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = worker.pushPage(context.Background()); err != nil {
				t.Fatal(err)
			}
			_, err = worker.pushPage(context.Background())
			if (err != nil) != (failure != "") {
				t.Fatalf("disposition result=%v", err)
			}
			if failure != "" {
				if _, err = worker.pushPage(context.Background()); err != nil {
					t.Fatal(err)
				}
				if requests.Load() != 2 {
					t.Fatalf("resent permanently rejected HTTP page: %d", requests.Load())
				}
			}
			receiverMu.Lock()
			retainedBodyMatches := len(store.pages) == 1 && bytes.Equal(store.pages[0].Payload, rejectedBody)
			receiverMu.Unlock()
			if !retainedBodyMatches || store.delivered != 1 || store.cursor.AssetSampleID != 2 || store.pages[0].StatusCode != 400 {
				t.Fatalf("rejected page not retained separately from delivery: %+v", store)
			}
			// A replacement worker resumes after the durable disposition and sends
			// T3, even though T1 arrived after T2 and was rejected by the receiver.
			worker, err = NewWorker(config, source, store)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = worker.pushPage(context.Background()); err != nil {
				t.Fatal(err)
			}
			receiverMu.Lock()
			actualTimestamp := lastTimestamp
			receiverMu.Unlock()
			if requests.Load() != 3 || store.cursor.AssetSampleID != 3 || store.delivered != 2 || actualTimestamp != 3000 {
				t.Fatalf("later sample stalled: requests=%d cursor=%+v delivered=%d time=%d", requests.Load(), store.cursor, store.delivered, actualTimestamp)
			}
		})
	}
}

func TestWorkerRetryableReceiverFailuresKeepCursor(t *testing.T) {
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")
	for _, status := range []int{429, 503} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if requests.Add(1) <= 2 {
					w.WriteHeader(status)
					return
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			defer server.Close()
			store := &rejectionCursor{}
			worker, err := NewWorker(Config{Enabled: true, URL: server.URL, Interval: MinInterval}, rejectedPageSource{[]int64{1000}}, store)
			if err != nil {
				t.Fatal(err)
			}
			for range 2 {
				if err = worker.pushBatch(context.Background()); err == nil || store.cursor != (Cursor{}) || store.calls != 0 {
					t.Fatalf("retryable failure consumed page: %v/%+v", err, store)
				}
			}
			if err = worker.pushBatch(context.Background()); err != nil || store.cursor.AssetSampleID != 1 || requests.Load() != 3 {
				t.Fatalf("retry did not recover: %v/%+v", err, store)
			}
		})
	}
}

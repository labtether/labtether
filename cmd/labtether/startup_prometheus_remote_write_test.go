package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/runtimesettings"
	"github.com/labtether/labtether/internal/telemetry/remotewrite"
)

type runtimeRemoteWriteStore struct {
	mu          sync.Mutex
	fingerprint string
	cursor      remotewrite.Cursor
	sample      remotewrite.SampleWithLabels
}

func (s *runtimeRemoteWriteStore) SamplesAfter(_ context.Context, cursor remotewrite.Cursor, _ int) (remotewrite.Batch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cursor.AssetSampleID >= 1 {
		return remotewrite.Batch{Next: cursor}, nil
	}
	return remotewrite.Batch{Samples: []remotewrite.SampleWithLabels{s.sample}, Next: remotewrite.Cursor{AssetSampleID: 1}}, nil
}

func (s *runtimeRemoteWriteStore) LoadRemoteWriteCursor(_ context.Context, fingerprint string) (remotewrite.Cursor, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fingerprint != fingerprint {
		s.fingerprint = fingerprint
		s.cursor = remotewrite.Cursor{}
	}
	return s.cursor, nil
}

func (s *runtimeRemoteWriteStore) SaveRemoteWriteCursor(_ context.Context, fingerprint string, cursor remotewrite.Cursor, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fingerprint != fingerprint {
		return context.Canceled
	}
	s.cursor = cursor
	return nil
}

func TestPrometheusRemoteWriteRuntimeAppliesEnableEndpointChangeAndDisable(t *testing.T) {
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")
	firstRequests := make(chan struct{}, 2)
	secondRequests := make(chan struct{}, 2)
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		firstRequests <- struct{}{}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		secondRequests <- struct{}{}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer second.Close()

	settings := persistence.NewMemoryRuntimeSettingsStore()
	data := &runtimeRemoteWriteStore{sample: remotewrite.SampleWithLabels{
		Labels: map[string]string{"__name__": "labtether_cpu_used_percent", "asset_id": "asset-1"},
		Value:  1, Timestamp: time.Now().UnixMilli(),
	}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runtime := newPrometheusRemoteWriteRuntime(ctx, settings, nil, data, data)
	defer runtime.Stop()
	if err := runtime.Apply(); err != nil {
		t.Fatalf("initial disabled Apply: %v", err)
	}
	_, _ = settings.SaveRuntimeSettingOverrides(map[string]string{
		runtimesettings.KeyPrometheusRemoteWriteEnabled:  "true",
		runtimesettings.KeyPrometheusRemoteWriteURL:      first.URL,
		runtimesettings.KeyPrometheusRemoteWriteInterval: "10s",
	})
	if err := runtime.Apply(); err != nil {
		t.Fatalf("enable Apply: %v", err)
	}
	awaitRemoteWriteRequest(t, firstRequests, "first endpoint")

	_, _ = settings.SaveRuntimeSettingOverrides(map[string]string{
		runtimesettings.KeyPrometheusRemoteWriteURL: second.URL,
	})
	if err := runtime.Apply(); err != nil {
		t.Fatalf("endpoint replacement Apply: %v", err)
	}
	awaitRemoteWriteRequest(t, secondRequests, "replacement endpoint")

	_, _ = settings.SaveRuntimeSettingOverrides(map[string]string{
		runtimesettings.KeyPrometheusRemoteWriteEnabled: "false",
	})
	if err := runtime.Apply(); err != nil {
		t.Fatalf("disable Apply: %v", err)
	}
	select {
	case <-secondRequests:
		t.Fatal("disabled runtime dispatched another request")
	case <-time.After(100 * time.Millisecond):
	}
}

func awaitRemoteWriteRequest(t *testing.T, requests <-chan struct{}, label string) {
	t.Helper()
	select {
	case <-requests:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s request", label)
	}
}

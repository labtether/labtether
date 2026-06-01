package collectors

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/hubcollector"
)

type hubCollectorHandlerStore struct {
	collector hubcollector.Collector
}

func (s *hubCollectorHandlerStore) CreateHubCollector(req hubcollector.CreateCollectorRequest) (hubcollector.Collector, error) {
	interval, err := hubcollector.CreateIntervalSeconds(req.IntervalSeconds)
	if err != nil {
		return hubcollector.Collector{}, err
	}
	s.collector = hubcollector.Collector{
		ID:              "collector-1",
		AssetID:         req.AssetID,
		CollectorType:   req.CollectorType,
		IntervalSeconds: interval,
		Enabled:         true,
	}
	return s.collector, nil
}

func (s *hubCollectorHandlerStore) GetHubCollector(id string) (hubcollector.Collector, bool, error) {
	if s.collector.ID == id {
		return s.collector, true, nil
	}
	return hubcollector.Collector{}, false, nil
}

func (s *hubCollectorHandlerStore) ListHubCollectors(limit int, enabledOnly bool) ([]hubcollector.Collector, error) {
	return nil, nil
}

func (s *hubCollectorHandlerStore) UpdateHubCollector(id string, req hubcollector.UpdateCollectorRequest) (hubcollector.Collector, error) {
	if s.collector.ID != id {
		return hubcollector.Collector{}, hubcollector.ErrCollectorNotFound
	}
	if req.IntervalSeconds != nil {
		if err := hubcollector.ValidateIntervalSeconds(*req.IntervalSeconds); err != nil {
			return hubcollector.Collector{}, err
		}
		s.collector.IntervalSeconds = *req.IntervalSeconds
	}
	return s.collector, nil
}

func (s *hubCollectorHandlerStore) DeleteHubCollector(id string) error { return nil }

func (s *hubCollectorHandlerStore) UpdateHubCollectorStatus(id, status, lastError string, collectedAt time.Time) error {
	return nil
}

func newHubCollectorHandlerDeps(store *hubCollectorHandlerStore) *Deps {
	return &Deps{
		HubCollectorStore: store,
		EnforceRateLimit: func(w http.ResponseWriter, r *http.Request, bucket string, limit int, window time.Duration) bool {
			return true
		},
	}
}

func TestHandleHubCollectorsRejectsOutOfRangeIntervalOnCreate(t *testing.T) {
	deps := newHubCollectorHandlerDeps(&hubCollectorHandlerStore{})
	payload := []byte(`{"asset_id":"asset-1","collector_type":"ssh","interval_seconds":2147483648}`)
	req := httptest.NewRequest(http.MethodPost, "/hub-collectors", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	deps.HandleHubCollectors(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleHubCollectorActionsRejectsOutOfRangeIntervalOnUpdate(t *testing.T) {
	store := &hubCollectorHandlerStore{
		collector: hubcollector.Collector{
			ID:              "collector-1",
			AssetID:         "asset-1",
			CollectorType:   hubcollector.CollectorTypeSSH,
			IntervalSeconds: hubcollector.DefaultIntervalSeconds,
		},
	}
	deps := newHubCollectorHandlerDeps(store)
	payload := []byte(`{"interval_seconds":2147483648}`)
	req := httptest.NewRequest(http.MethodPatch, "/hub-collectors/collector-1", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	deps.HandleHubCollectorActions(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

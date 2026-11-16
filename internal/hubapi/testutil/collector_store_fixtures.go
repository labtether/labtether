package testutil

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/hubcollector"
)

// ErrorHubCollectorStore is a hub collector store over a fixed list that
// returns ListErr or GetErr when set. It is the exported copy of
// cmd/labtether's errorHubCollectorStore; cmd keeps its own for the tests
// that still build struct literals with the unexported fields.
type ErrorHubCollectorStore struct {
	Collectors []hubcollector.Collector
	ListErr    error
	GetErr     error
}

func (s *ErrorHubCollectorStore) CreateHubCollector(hubcollector.CreateCollectorRequest) (hubcollector.Collector, error) {
	return hubcollector.Collector{}, fmt.Errorf("not implemented")
}

func (s *ErrorHubCollectorStore) GetHubCollector(id string) (hubcollector.Collector, bool, error) {
	if s.GetErr != nil {
		return hubcollector.Collector{}, false, s.GetErr
	}
	for _, collector := range s.Collectors {
		if collector.ID == id {
			return collector, true, nil
		}
	}
	return hubcollector.Collector{}, false, nil
}

func (s *ErrorHubCollectorStore) ListHubCollectors(_ int, enabledOnly bool) ([]hubcollector.Collector, error) {
	if s.ListErr != nil {
		return nil, s.ListErr
	}
	out := make([]hubcollector.Collector, 0, len(s.Collectors))
	for _, collector := range s.Collectors {
		if enabledOnly && !collector.Enabled {
			continue
		}
		out = append(out, collector)
	}
	return out, nil
}

func (s *ErrorHubCollectorStore) UpdateHubCollector(string, hubcollector.UpdateCollectorRequest) (hubcollector.Collector, error) {
	return hubcollector.Collector{}, fmt.Errorf("not implemented")
}

func (s *ErrorHubCollectorStore) DeleteHubCollector(string) error {
	return fmt.Errorf("not implemented")
}

func (s *ErrorHubCollectorStore) UpdateHubCollectorStatus(string, string, string, time.Time) error {
	return nil
}

// FindEnabledHubCollectorsByType is the bounded lookup, as on
// StubHubCollectorStore; it fails with ListErr when set.
func (s *ErrorHubCollectorStore) FindEnabledHubCollectorsByType(kind string) ([]hubcollector.Collector, error) {
	if s.ListErr != nil {
		return nil, s.ListErr
	}
	return enabledCollectorsOfType(s.Collectors, kind), nil
}

// enabledCollectorsOfType returns at most two enabled collectors of kind, as
// the PostgreSQL lookup does (two are enough to refuse an ambiguous owner).
// It is cmd's enabledRuntimeFixtureCollectors.
func enabledCollectorsOfType(collectors []hubcollector.Collector, kind string) []hubcollector.Collector {
	var found []hubcollector.Collector
	for _, collector := range collectors {
		if collector.Enabled && collector.CollectorType == kind {
			found = append(found, collector)
			if len(found) == 2 {
				break
			}
		}
	}
	return found
}

// AssertErrorBodyContains fails the test unless body is a JSON object whose
// "error" string contains want.
func AssertErrorBodyContains(t *testing.T, body []byte, want string) {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode error payload: %v", err)
	}
	raw, _ := payload["error"].(string)
	if !strings.Contains(raw, want) {
		t.Fatalf("expected error containing %q, got %q", want, raw)
	}
}

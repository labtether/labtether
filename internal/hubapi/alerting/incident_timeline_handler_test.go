package alerting

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/incidents"
)

type recordingIncidentEventStore struct {
	events             []incidents.IncidentEvent
	err                error
	listCalls          int
	receivedIncidentID string
	receivedLimit      int
}

func (s *recordingIncidentEventStore) UpsertIncidentEvent(req incidents.CreateIncidentEventRequest) (incidents.IncidentEvent, error) {
	return incidents.IncidentEvent{}, errors.New("unexpected UpsertIncidentEvent call")
}

func (s *recordingIncidentEventStore) ListIncidentEvents(incidentID string, limit int) ([]incidents.IncidentEvent, error) {
	s.listCalls++
	s.receivedIncidentID = incidentID
	s.receivedLimit = limit
	return s.events, s.err
}

func createTimelineTestIncident(t *testing.T, deps *Deps) incidents.Incident {
	t.Helper()
	incident, err := deps.IncidentStore.CreateIncident(incidents.CreateIncidentRequest{
		Title:    "Timeline test incident",
		Severity: incidents.SeverityHigh,
	})
	if err != nil {
		t.Fatalf("create incident: %v", err)
	}
	return incident
}

func TestHandleIncidentActionsTimelineReturnsEventsAndForwardsLimit(t *testing.T) {
	deps := newTestAlertingDeps(t)
	incident := createTimelineTestIncident(t, deps)
	occurredAt := time.Date(2026, time.July, 13, 3, 4, 5, 0, time.UTC)
	createdAt := occurredAt.Add(time.Second)
	eventStore := &recordingIncidentEventStore{events: []incidents.IncidentEvent{
		{
			ID:         "ievt-1",
			IncidentID: incident.ID,
			EventType:  incidents.EventTypeAlertFired,
			SourceRef:  "alert-instance-1",
			Summary:    "CPU threshold exceeded",
			Severity:   "critical",
			Metadata:   map[string]any{"asset_id": "asset-1"},
			OccurredAt: occurredAt,
			CreatedAt:  createdAt,
		},
	}}
	deps.IncidentEventStore = eventStore

	req := httptest.NewRequest(http.MethodGet, "/incidents/"+incident.ID+"/timeline?limit=7", nil)
	rec := httptest.NewRecorder()
	deps.HandleIncidentActions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if eventStore.listCalls != 1 {
		t.Fatalf("expected one event-store call, got %d", eventStore.listCalls)
	}
	if eventStore.receivedIncidentID != incident.ID {
		t.Fatalf("expected incident id %q, got %q", incident.ID, eventStore.receivedIncidentID)
	}
	if eventStore.receivedLimit != 7 {
		t.Fatalf("expected limit 7, got %d", eventStore.receivedLimit)
	}

	var response struct {
		Events []incidents.IncidentEvent `json:"events"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode timeline response: %v", err)
	}
	if len(response.Events) != 1 {
		t.Fatalf("expected one event, got %d", len(response.Events))
	}
	got := response.Events[0]
	if got.ID != "ievt-1" || got.EventType != incidents.EventTypeAlertFired || got.SourceRef != "alert-instance-1" {
		t.Fatalf("unexpected event identity fields: %#v", got)
	}
	if got.Summary != "CPU threshold exceeded" || got.Severity != "critical" {
		t.Fatalf("unexpected event display fields: %#v", got)
	}
	if !got.OccurredAt.Equal(occurredAt) || !got.CreatedAt.Equal(createdAt) {
		t.Fatalf("unexpected event timestamps: occurred=%s created=%s", got.OccurredAt, got.CreatedAt)
	}
}

func TestHandleIncidentActionsTimelineRejectsMissingIncidentBeforeListingEvents(t *testing.T) {
	deps := newTestAlertingDeps(t)
	eventStore := &recordingIncidentEventStore{}
	deps.IncidentEventStore = eventStore

	req := httptest.NewRequest(http.MethodGet, "/incidents/inc-missing/timeline", nil)
	rec := httptest.NewRecorder()
	deps.HandleIncidentActions(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
	}
	if eventStore.listCalls != 0 {
		t.Fatalf("expected no event-store call for a missing incident, got %d", eventStore.listCalls)
	}
}

func TestHandleIncidentActionsTimelineHandlesUnavailableAndFailedEventStore(t *testing.T) {
	t.Run("unavailable", func(t *testing.T) {
		deps := newTestAlertingDeps(t)
		incident := createTimelineTestIncident(t, deps)

		req := httptest.NewRequest(http.MethodGet, "/incidents/"+incident.ID+"/timeline", nil)
		rec := httptest.NewRecorder()
		deps.HandleIncidentActions(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("list failure", func(t *testing.T) {
		deps := newTestAlertingDeps(t)
		incident := createTimelineTestIncident(t, deps)
		deps.IncidentEventStore = &recordingIncidentEventStore{err: errors.New("database unavailable")}

		req := httptest.NewRequest(http.MethodGet, "/incidents/"+incident.ID+"/timeline", nil)
		rec := httptest.NewRecorder()
		deps.HandleIncidentActions(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d body=%s", rec.Code, rec.Body.String())
		}
	})
}

func TestHandleIncidentActionsTimelineAllowsOnlyGet(t *testing.T) {
	deps := newTestAlertingDeps(t)
	incident := createTimelineTestIncident(t, deps)
	deps.IncidentEventStore = &recordingIncidentEventStore{}

	req := httptest.NewRequest(http.MethodPost, "/incidents/"+incident.ID+"/timeline", nil)
	rec := httptest.NewRecorder()
	deps.HandleIncidentActions(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d body=%s", rec.Code, rec.Body.String())
	}
}

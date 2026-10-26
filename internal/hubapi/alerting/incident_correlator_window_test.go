package alerting

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/incidents"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/updates"
)

type correlatedTimelineStore struct{ recordingIncidentEventStore }

func (s *correlatedTimelineStore) UpsertIncidentEvent(req incidents.CreateIncidentEventRequest) (incidents.IncidentEvent, error) {
	for _, event := range s.events {
		if event.IncidentID == req.IncidentID && event.SourceRef == req.SourceRef {
			return event, nil
		}
	}
	event := incidents.IncidentEvent{IncidentID: req.IncidentID, EventType: req.EventType,
		SourceRef: req.SourceRef, Summary: req.Summary, Severity: req.Severity, OccurredAt: req.OccurredAt}
	s.events = append(s.events, event)
	return event, nil
}

func (s *correlatedTimelineStore) ListIncidentEvents(incidentID string, limit int) ([]incidents.IncidentEvent, error) {
	var result []incidents.IncidentEvent
	for _, event := range s.events {
		if event.IncidentID == incidentID {
			result = append(result, event)
			if len(result) == limit {
				break
			}
		}
	}
	return result, nil
}

func TestIncidentCorrelationKeepsWindowEventsBehindRecentlyUpdatedOldRows(t *testing.T) {
	deps := newTestAlertingDeps(t)
	incident := createTimelineTestIncident(t, deps)
	events := &correlatedTimelineStore{}
	deps.IncidentEventStore = events
	now := time.Now().UTC()
	incident.OpenedAt = now.Add(-time.Hour)
	incident.PrimaryAssetID = "window-node"
	old, inside, future := now.Add(-2*time.Hour), now.Add(-time.Minute), now.Add(time.Hour)
	firingScope := &alerts.FiringScope{Version: 1, AssetIDs: []string{"window-node"}}
	failedTarget := []updates.RunResultEntry{{Target: "window-node", Status: updates.StatusFailed}}
	// Each supplied page follows the PostgreSQL stores' updated_at DESC order.
	// Creation/start time deliberately has a different order in the same page.
	page := correlationPage{
		actionRuns: []actions.Run{
			{ID: "old", Target: "window-node", Status: actions.StatusFailed, CreatedAt: old, UpdatedAt: now},
			{ID: "inside", Target: "window-node", Status: actions.StatusFailed, CreatedAt: inside, UpdatedAt: now.Add(-time.Second)},
			{ID: "future", Target: "window-node", Status: actions.StatusFailed, CreatedAt: future, UpdatedAt: now.Add(-2 * time.Second)},
		},
		updateRuns: []updates.Run{
			{ID: "old", Status: updates.StatusFailed, Results: failedTarget, CreatedAt: old, UpdatedAt: now},
			{ID: "inside", Status: updates.StatusFailed, Results: failedTarget, CreatedAt: inside, UpdatedAt: now.Add(-time.Second)},
			{ID: "future", Status: updates.StatusFailed, Results: failedTarget, CreatedAt: future, UpdatedAt: now.Add(-2 * time.Second)},
		},
		alertInstances: []alerts.AlertInstance{
			{ID: "old", FirstFiredAt: &old, FiringScope: firingScope, StartedAt: old, UpdatedAt: now},
			{ID: "inside", FirstFiredAt: &inside, FiringScope: firingScope, StartedAt: old, UpdatedAt: now.Add(-time.Second)},
			{ID: "future", FirstFiredAt: &future, FiringScope: firingScope, StartedAt: inside, UpdatedAt: now.Add(-2 * time.Second)},
		},
	}
	deps.correlateIncident(incident, page)
	rec := httptest.NewRecorder()
	deps.HandleIncidentActions(rec, httptest.NewRequest(http.MethodGet, "/incidents/"+incident.ID+"/timeline", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("timeline: %d %s", rec.Code, rec.Body.String())
	}
	var result struct {
		Events []incidents.IncidentEvent `json:"events"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"action_runs:inside": incidents.EventTypeActionRun,
		"update_runs:inside": incidents.EventTypeUpdateRun, "alert_instances:inside": incidents.EventTypeAlertFired}
	if len(result.Events) != len(want) {
		t.Fatalf("timeline events=%+v, want exactly the three in-window records", result.Events)
	}
	for _, event := range result.Events {
		if want[event.SourceRef] != event.EventType || event.IncidentID != incident.ID || !event.OccurredAt.Equal(inside) {
			t.Fatalf("unexpected correlated event: %+v", event)
		}
		delete(want, event.SourceRef)
	}
	if len(want) != 0 {
		t.Fatalf("missing timeline events: %v", want)
	}
}

// Existing event-matcher cases use the same scoped page owner as the worker.
func (d *Deps) correlateIncident(inc incidents.Incident, page correlationPage) {
	ctx, cancel := context.WithTimeout(context.Background(), incidentCorrelationSlice)
	defer cancel()
	scope, err := d.incidentCorrelationScope(ctx, inc)
	if err != nil {
		return
	}
	_ = d.correlateIncidentPage(ctx, inc, scope, page, persistence.CorrelationWindow{From: inc.OpenedAt, Through: time.Now().UTC()})
}

func (s *correlatedTimelineStore) UpsertIncidentEventContext(ctx context.Context, req incidents.CreateIncidentEventRequest) (incidents.IncidentEvent, error) {
	if err := ctx.Err(); err != nil {
		return incidents.IncidentEvent{}, err
	}
	return s.UpsertIncidentEvent(req)
}

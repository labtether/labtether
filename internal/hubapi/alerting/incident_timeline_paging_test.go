package alerting

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/incidents"
	"github.com/labtether/labtether/internal/persistence"
)

type timelinePagingStore struct {
	recordingIncidentEventStore
	read func(context.Context, string, persistence.IncidentEventPageCursor, int) (persistence.IncidentEventPage, error)
}

func (s *timelinePagingStore) ListIncidentEventPage(ctx context.Context, id string, cursor persistence.IncidentEventPageCursor, limit int) (persistence.IncidentEventPage, error) {
	return s.read(ctx, id, cursor, limit)
}

func TestIncidentTimelineCursorStaysBoundToIncidentAndCurrentAccess(t *testing.T) {
	d := newTestAlertingDeps(t)
	incident := createTimelineTestIncident(t, d)
	other := createTimelineTestIncident(t, d)
	at := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	next := persistence.IncidentEventPageCursor{IncidentID: incident.ID, ThroughCreatedAt: at.Add(time.Hour), BeforeOccurredAt: at, BeforeID: "ievt-page-1"}
	calls := 0
	store := &timelinePagingStore{}
	store.read = func(ctx context.Context, id string, cursor persistence.IncidentEventPageCursor, limit int) (persistence.IncidentEventPage, error) {
		calls++
		if id != incident.ID || limit != 500 {
			t.Fatalf("wrong owner or unbounded page: %s/%d", id, limit)
		}
		if calls == 1 {
			if cursor != (persistence.IncidentEventPageCursor{}) {
				t.Fatal("first page has a continuation")
			}
			return persistence.IncidentEventPage{Events: []incidents.IncidentEvent{{ID: next.BeforeID, IncidentID: id, EventType: incidents.EventTypeAlertFired, Summary: "Actual incident event", OccurredAt: at, CreatedAt: at}}, Next: &next}, nil
		}
		if cursor != next {
			t.Fatal("continuation changed its owner, order key or cutoff")
		}
		return persistence.IncidentEventPage{Events: []incidents.IncidentEvent{}}, nil
	}
	d.IncidentEventStore = store
	request := func(id, cursor string, restricted bool) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(http.MethodGet, "/incidents/"+id+"/timeline?limit=999&cursor="+url.QueryEscape(cursor), nil)
		if restricted {
			r = r.WithContext(apiv2.ContextWithAllowedAssets(r.Context(), []string{"unrelated-asset"}))
		}
		w := httptest.NewRecorder()
		d.HandleIncidentActions(w, r)
		return w
	}
	first := request(incident.ID, "", false)
	var payload struct {
		Events []incidents.IncidentEvent `json:"events"`
		Next   string                    `json:"next_cursor"`
	}
	if first.Code != 200 || json.Unmarshal(first.Body.Bytes(), &payload) != nil || len(payload.Events) != 1 || payload.Events[0].Summary != "Actual incident event" || payload.Next == "" {
		t.Fatalf("first page lost canonical events or continuation: %d %s", first.Code, first.Body.String())
	}
	for _, item := range []struct {
		id, cursor string
		restricted bool
		want       int
	}{
		{other.ID, payload.Next, false, 400},
		{incident.ID, "broken", false, 400},
		{incident.ID, strings.Repeat("x", 4097), false, 400},
		{incident.ID, payload.Next, true, 403},
		{"missing-incident", payload.Next, false, 404},
	} {
		if got := request(item.id, item.cursor, item.restricted); got.Code != item.want {
			t.Fatalf("cursor/access rejection: %d/%d %s", got.Code, item.want, got.Body.String())
		}
	}
	if calls != 1 {
		t.Fatal("rejected cursor or revoked scope reached the history store")
	}
	second := request(incident.ID, payload.Next, false)
	if second.Code != 200 || json.Unmarshal(second.Body.Bytes(), &payload) != nil || len(payload.Events) != 0 || payload.Next != "" || calls != 2 {
		t.Fatalf("continuation did not reach true EOF: %d %s", second.Code, second.Body.String())
	}
	if store.listCalls != 0 {
		t.Fatal("paged owner fell back to a capped legacy list")
	}
}

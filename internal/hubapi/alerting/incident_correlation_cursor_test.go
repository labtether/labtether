package alerting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/incidents"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/updates"
)

// Protect retained older incidents and events against the former newest-100
// cutoffs, and protect successful effects against cancellation/failure replay.
func TestIncidentCorrelationPagesRetainedHistoryAndResumesFailedEffects(t *testing.T) {
	t.Run("older active incidents and four scoped sources", func(t *testing.T) {
		d := newTestAlertingDeps(t)
		sink := &correlatedTimelineStore{}
		d.IncidentEventStore = sink
		var all []incidents.Incident
		for i := 0; i < 202; i++ {
			inc := createPagingIncident(t, d, "shared")
			if i%2 == 1 {
				state := incidents.StatusInvestigating
				var err error
				inc, err = d.IncidentStore.UpdateIncident(inc.ID, incidents.UpdateIncidentRequest{Status: &state})
				if err != nil {
					t.Fatal(err)
				}
			}
			all = append(all, inc)
		}
		wanted := createPagingSources(t, d, "shared", "owned")
		for i := 0; i < 131; i++ {
			createPagingSources(t, d, "unrelated", fmt.Sprintf("new-%03d", i))
		}
		// All old incident IDs survive updated_at reordering and the two status
		// populations both exceed the original separate 100-row inventories.
		title := "edited while retained"
		if _, err := d.IncidentStore.UpdateIncident(all[0].ID, incidents.UpdateIncidentRequest{Title: &title}); err != nil {
			t.Fatal(err)
		}
		d.correlateOpenIncidents(context.Background())
		for _, inc := range all {
			assertPagingTimeline(t, d, inc.ID, wanted)
		}
		d.correlateOpenIncidents(context.Background())
		for _, inc := range all {
			assertPagingTimeline(t, d, inc.ID, wanted)
		}
	})
	t.Run("failed row retries while another incident progresses", func(t *testing.T) {
		d, main, other, ids := pagingActionFixture(t)
		sink := &pagingEffectStore{failRef: "action_runs:" + ids[1]}
		d.IncidentEventStore = sink
		d.correlateOpenIncidents(context.Background())
		if sink.failures != 1 {
			t.Fatalf("expected one attempted failure, got %d", sink.failures)
		}
		assertPagingTimeline(t, d, other.ID, map[string]bool{"action_runs:" + ids[len(ids)-1]: true})
		if sink.has(main.ID, sink.failRef) {
			t.Fatal("failed effect was treated as stored")
		}
		entry := d.incidentCorrelationWork.entries[main.ID].Value.(*incidentCorrelationSweep)
		if entry.cursors[0].AfterID != ids[0] {
			t.Fatalf("cursor advanced past failed row: %+v", entry.cursors[0])
		}
		sink.failRef = ""
		d.correlateOpenIncidents(context.Background())
		want := make(map[string]bool)
		for _, id := range ids[:len(ids)-1] {
			want["action_runs:"+id] = true
		}
		assertPagingTimeline(t, d, main.ID, want)
	})
	t.Run("cancel after successful write replays without duplicates", func(t *testing.T) {
		d, main, _, ids := pagingActionFixture(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		sink := &pagingEffectStore{cancel: cancel, cancelRef: "action_runs:" + ids[1]}
		d.IncidentEventStore = sink
		d.correlateOpenIncidents(ctx)
		if !sink.has(main.ID, sink.cancelRef) {
			t.Fatal("cancellation did not follow the selected real write")
		}
		entry := d.incidentCorrelationWork.entries[main.ID].Value.(*incidentCorrelationSweep)
		if entry.cursors[0].AfterID != ids[0] {
			t.Fatal("uncertain effect advanced cursor")
		}
		d.correlateOpenIncidents(context.Background())
		want := make(map[string]bool)
		for _, id := range ids[:len(ids)-1] {
			want["action_runs:"+id] = true
		}
		assertPagingTimeline(t, d, main.ID, want)
		if sink.cancelAttempts != 2 {
			t.Fatalf("uncertain row attempts=%d, want2", sink.cancelAttempts)
		}
	})
}

func createPagingIncident(t *testing.T, d *Deps, target string) incidents.Incident {
	t.Helper()
	inc, err := d.IncidentStore.CreateIncident(incidents.CreateIncidentRequest{Title: "Paged correlation", PrimaryAssetID: target})
	if err != nil {
		t.Fatal(err)
	}
	return inc
}

func createPagingAction(t *testing.T, d *Deps, target string) string {
	t.Helper()
	run, err := d.ActionStore.CreateActionRun(actions.ExecuteRequest{Target: target, Command: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.ActionStore.ApplyActionResult(actions.Result{RunID: run.ID, Status: actions.StatusFailed}); err != nil {
		t.Fatal(err)
	}
	return run.ID
}

func createPagingSources(t *testing.T, d *Deps, target, name string) map[string]bool {
	t.Helper()
	want := map[string]bool{"action_runs:" + createPagingAction(t, d, target): true}
	plan, err := d.UpdateStore.CreateUpdatePlan(updates.CreatePlanRequest{Name: name, Targets: []string{target}})
	if err != nil {
		t.Fatal(err)
	}
	run, err := d.UpdateStore.CreateUpdateRun(plan, updates.ExecutePlanRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.UpdateStore.ApplyUpdateResult(updates.Result{RunID: run.ID, Status: updates.StatusFailed,
		Results: []updates.RunResultEntry{{Target: target, Status: updates.StatusFailed}}}); err != nil {
		t.Fatal(err)
	}
	want["update_runs:"+run.ID] = true
	instance, err := d.AlertInstanceStore.CreateAlertInstance(alerts.CreateInstanceRequest{RuleID: "rule-" + name})
	if err != nil {
		t.Fatal(err)
	}
	owner := d.AlertInstanceStore.(*persistence.MemoryAlertInstanceStore)
	if _, err := owner.FireAlertInstance(context.Background(), instance.ID, alerts.FiringScope{Version: 1, AssetIDs: []string{target}}); err != nil {
		t.Fatal(err)
	}
	want["alert_instances:"+instance.ID] = true
	id := "paging-audit-" + name
	if err := d.AuditStore.Append(audit.Event{ID: id, Type: "fixture", ActorID: "fixture", Target: target, Timestamp: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	want["audit_events:"+id] = true
	return want
}

func pagingActionFixture(t *testing.T) (*Deps, incidents.Incident, incidents.Incident, []string) {
	t.Helper()
	d := newTestAlertingDeps(t)
	main, other := createPagingIncident(t, d, "large"), createPagingIncident(t, d, "other")
	var ids []string
	for i := 0; i < 129; i++ {
		ids = append(ids, createPagingAction(t, d, "large"))
	}
	sort.Strings(ids)
	// The final slice slot denotes the other incident's action, not sort order.
	ids = append(ids, createPagingAction(t, d, "other"))
	return d, main, other, ids
}

func assertPagingTimeline(t *testing.T, d *Deps, id string, want map[string]bool) {
	t.Helper()
	rec := httptest.NewRecorder()
	d.HandleIncidentActions(rec, httptest.NewRequest(http.MethodGet, "/incidents/"+id+"/timeline?limit=500", nil))
	var reply struct {
		Events []incidents.IncidentEvent `json:"events"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &reply) != nil {
		t.Fatalf("timeline: %d %s", rec.Code, rec.Body.String())
	}
	if len(reply.Events) != len(want) {
		t.Fatalf("incident %s events=%d want=%d", id, len(reply.Events), len(want))
	}
	seen := make(map[string]bool)
	for _, event := range reply.Events {
		if event.IncidentID != id || !want[event.SourceRef] || seen[event.SourceRef] {
			t.Fatalf("wrong or repeated event: %+v", event)
		}
		seen[event.SourceRef] = true
	}
}

type pagingEffectStore struct {
	correlatedTimelineStore
	failRef, cancelRef       string
	failures, cancelAttempts int
	cancel                   context.CancelFunc
}

func (s *pagingEffectStore) UpsertIncidentEventContext(ctx context.Context, req incidents.CreateIncidentEventRequest) (incidents.IncidentEvent, error) {
	if req.SourceRef == s.failRef {
		s.failures++
		return incidents.IncidentEvent{}, errors.New("injected write failure")
	}
	event, err := s.correlatedTimelineStore.UpsertIncidentEventContext(ctx, req)
	if req.SourceRef == s.cancelRef && err == nil {
		s.cancelAttempts++
		if s.cancel != nil {
			s.cancel()
			s.cancel = nil
		}
	}
	return event, err
}

func (s *pagingEffectStore) has(id, ref string) bool {
	for _, event := range s.events {
		if event.IncidentID == id && event.SourceRef == ref {
			return true
		}
	}
	return false
}

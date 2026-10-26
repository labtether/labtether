package alerting

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/incidents"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/updates"
)

// Interleaved sources must stay with their own incident. A failed multi-target
// update must not implicate its successful target, and current group edits
// must not retarget an old alert's immutable firing witness.
func TestIncidentCorrelationScopesSourcesAndKnownFireHistory(t *testing.T) {
	d := newTestAlertingDeps(t)
	d.IncidentEventStore = &correlatedTimelineStore{}
	links := &authorizationDependencyStore{incidentAssets: make(map[string][]incidents.IncidentAsset)}
	d.DependencyStore = links
	now := time.Now().UTC()
	fired := now.Add(-time.Minute)
	makeIncident := func(asset, group string) incidents.Incident {
		t.Helper()
		inc, err := d.IncidentStore.CreateIncident(incidents.CreateIncidentRequest{Title: "Scoped incident", Severity: "high", PrimaryAssetID: asset, GroupID: group})
		if err != nil {
			t.Fatal(err)
		}
		inc.OpenedAt = now.Add(-time.Hour)
		return inc
	}
	a, b, group := makeIncident("asset-a", ""), makeIncident("asset-b", ""), makeIncident("", "group-current")
	linked, unscoped, explicit := makeIncident("", ""), makeIncident("", ""), makeIncident("", "")
	if _, err := links.LinkIncidentAsset(linked.ID, incidents.LinkAssetRequest{AssetID: "asset-c", Role: incidents.AssetRoleRelated}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.AssetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{AssetID: "asset-c", Type: "host", Name: "C", Source: "agent", GroupID: "group-current"}); err != nil {
		t.Fatal(err)
	}
	link := incidents.LinkAlertRequest{AlertRuleID: "legacy-rule", AlertInstanceID: "legacy-fire", AlertFingerprint: "legacy-fingerprint", LinkType: incidents.LinkTypeTrigger}
	if _, err := d.IncidentStore.LinkIncidentAlert(explicit.ID, link); err != nil {
		t.Fatal(err)
	}
	instance := func(id, asset, groupID string) alerts.AlertInstance {
		scope := &alerts.FiringScope{Version: 1}
		if asset != "" {
			scope.AssetIDs = []string{asset}
		}
		if groupID != "" {
			scope.GroupIDs = []string{groupID}
		}
		return alerts.AlertInstance{ID: id, RuleID: "rule-" + id, Status: alerts.InstanceStatusResolved,
			StartedAt: now.Add(-2 * time.Hour), FirstFiredAt: &fired, FiringScope: scope}
	}
	legacy := alerts.AlertInstance{ID: link.AlertInstanceID, RuleID: link.AlertRuleID, Fingerprint: link.AlertFingerprint,
		Status: alerts.InstanceStatusResolved, FirstFiredAt: &fired}
	pending := instance("pending", "asset-a", "")
	pending.Status, pending.FirstFiredAt = alerts.InstanceStatusPending, nil
	unknown := instance("unknown", "asset-a", "")
	unknown.FirstFiredAt = nil
	invalid := instance("invalid", "asset-a", "")
	invalid.FiringScope.Version = 99
	page := correlationPage{
		actionRuns: []actions.Run{
			{ID: "a", Target: "asset-a", Status: actions.StatusFailed, CreatedAt: fired},
			{ID: "b", Target: "asset-b", Status: actions.StatusFailed, CreatedAt: fired},
			{ID: "c", Target: "asset-c", Status: actions.StatusFailed, CreatedAt: fired},
			{ID: "blank", Status: actions.StatusFailed, CreatedAt: fired},
			{ID: "success", Target: "asset-a", Status: actions.StatusSucceeded, CreatedAt: fired},
		},
		updateRuns: []updates.Run{
			{ID: "mixed", Status: updates.StatusFailed, CreatedAt: fired, Results: []updates.RunResultEntry{
				{Target: "asset-a", Status: updates.StatusSucceeded}, {Target: "asset-b", Status: updates.StatusFailed}}},
			{ID: "c", Status: updates.StatusFailed, CreatedAt: fired, Results: []updates.RunResultEntry{{Target: "asset-c", Status: updates.StatusFailed}}},
		},
		alertInstances: []alerts.AlertInstance{instance("a", "asset-a", ""), instance("b", "asset-b", ""),
			instance("old-group", "asset-c", "group-before-move"), instance("group", "", "group-current"), legacy, pending, unknown, invalid},
		auditEvents: []audit.Event{{ID: "a", Target: "asset-a", Timestamp: fired}, {ID: "b", Target: "asset-b", Timestamp: fired},
			{ID: "c", Target: "asset-c", Timestamp: fired}, {ID: "incident-a", Target: a.ID, Timestamp: fired}, {ID: "blank", Timestamp: fired}},
	}
	for _, inc := range []incidents.Incident{a, b, group, linked, unscoped, explicit} {
		d.correlateIncident(inc, page)
	}
	check := func(inc incidents.Incident, want ...string) {
		t.Helper()
		rec := httptest.NewRecorder()
		d.HandleIncidentActions(rec, httptest.NewRequest(http.MethodGet, "/incidents/"+inc.ID+"/timeline", nil))
		var response struct {
			Events []incidents.IncidentEvent `json:"events"`
		}
		if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &response) != nil {
			t.Fatalf("timeline: %d %s", rec.Code, rec.Body.String())
		}
		expected := map[string]bool{}
		for _, ref := range want {
			expected[ref] = true
		}
		if len(response.Events) != len(expected) {
			t.Fatalf("incident %s: got %+v, want %v", inc.ID, response.Events, want)
		}
		for _, event := range response.Events {
			if !expected[event.SourceRef] || event.IncidentID != inc.ID || !event.OccurredAt.Equal(fired) {
				t.Fatalf("wrong incident event: %+v", event)
			}
			delete(expected, event.SourceRef)
		}
	}
	check(a, "action_runs:a", "alert_instances:a", "audit_events:a", "audit_events:incident-a")
	check(b, "action_runs:b", "update_runs:mixed", "alert_instances:b", "audit_events:b")
	check(group, "action_runs:c", "update_runs:c", "alert_instances:group", "audit_events:c")
	check(linked, "action_runs:c", "update_runs:c", "alert_instances:old-group", "audit_events:c")
	check(unscoped)
	check(explicit, "alert_instances:legacy-fire")
	// Each exact link discriminator is mandatory; a known rule alone must not
	// admit another firing, and an explicit link cannot invent a legacy fire.
	for _, changed := range []string{"instance", "rule", "fingerprint", "unknown-fire"} {
		t.Run(changed, func(t *testing.T) {
			d.IncidentEventStore = &correlatedTimelineStore{}
			wrong := legacy
			switch changed {
			case "instance":
				wrong.ID = "another-fire"
			case "rule":
				wrong.RuleID = "another-rule"
			case "fingerprint":
				wrong.Fingerprint = "another-fingerprint"
			case "unknown-fire":
				wrong.FirstFiredAt = nil
			}
			d.correlateIncident(explicit, correlationPage{alertInstances: []alerts.AlertInstance{wrong}})
			check(explicit)
		})
	}
}

type failedCorrelationLinks struct{ persistence.IncidentStore }

func (failedCorrelationLinks) ReadIncidentAlertLinksContext(context.Context, string, func(incidents.AlertLink) error) error {
	return errors.New("scope read failed")
}

type failedCorrelationAssets struct {
	persistence.DependencyStore
	full bool
}

func (s failedCorrelationAssets) ReadIncidentAssetsContext(ctx context.Context, id string, visit func(incidents.IncidentAsset) error) error {
	if s.full {
		return visit(incidents.IncidentAsset{IncidentID: id, AssetID: " "})
	}
	return errors.New("asset scope read failed")
}

type failedCorrelationGroup struct{ persistence.AssetStore }

func (failedCorrelationGroup) ReadGroupAssetIDsContext(context.Context, string, func(string) error) error {
	return errors.New("group read failed")
}

func TestIncidentCorrelationScopeFailureNeverFallsBackToGlobal(t *testing.T) {
	for _, failure := range []string{"alert-links", "asset-links", "malformed-asset-link", "group-members"} {
		t.Run(failure, func(t *testing.T) {
			d := newTestAlertingDeps(t)
			inc := createTimelineTestIncident(t, d)
			inc.PrimaryAssetID = "owned"
			inc.OpenedAt = time.Now().Add(-time.Hour)
			events := &correlatedTimelineStore{}
			d.IncidentEventStore = events
			switch failure {
			case "alert-links":
				d.IncidentStore = failedCorrelationLinks{d.IncidentStore}
			case "asset-links":
				d.DependencyStore = failedCorrelationAssets{}
			case "malformed-asset-link":
				d.DependencyStore = failedCorrelationAssets{full: true}
			case "group-members":
				inc.GroupID = "group"
				d.AssetStore = failedCorrelationGroup{d.AssetStore}
			}
			d.correlateIncident(inc, correlationPage{actionRuns: []actions.Run{{ID: "owned-failure", Target: "owned", Status: actions.StatusFailed, CreatedAt: time.Now().Add(-time.Minute)}}})
			if len(events.events) != 0 {
				t.Fatal("scope failure still wrote an event")
			}
		})
	}
}

func (s *authorizationDependencyStore) ReadIncidentAssetsContext(ctx context.Context, id string, visit func(incidents.IncidentAsset) error) error {
	for _, row := range s.incidentAssets[id] {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := visit(row); err != nil {
			return err
		}
	}
	return ctx.Err()
}

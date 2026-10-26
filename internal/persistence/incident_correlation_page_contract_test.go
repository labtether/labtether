package persistence

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/incidents"
	"github.com/labtether/labtether/internal/updates"
)

type correlationPageFixture struct {
	prefix    string
	at        time.Time
	window    CorrelationWindow
	scope     IncidentCorrelationScope
	incidents []incidents.Incident
	actions   []actions.Run
	updates   []updates.Run
	alerts    []alerts.AlertInstance
	audits    []audit.Event
}

func newCorrelationPageFixture(prefix string) correlationPageFixture {
	f := correlationPageFixture{prefix: prefix, at: time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)}
	f.window = CorrelationWindow{From: f.at.Add(-time.Minute), Through: f.at.Add(time.Minute)}
	f.scope = IncidentCorrelationScope{IncidentID: prefix + "incident-000", GroupID: prefix + "group", AssetIDs: []string{prefix + "asset"}, OperationAssetIDs: []string{prefix + "asset", prefix + "asset-b"}}
	for i := 0; i < 129; i++ {
		status := "open"
		if i%2 == 1 {
			status = "investigating"
		}
		f.incidents = append(f.incidents, incidents.Incident{ID: fmt.Sprintf("%sincident-%03d", prefix, i), Status: status, OpenedAt: f.at, CreatedAt: f.at})
		id := fmt.Sprintf("%saction-%03d", prefix, i)
		if i == 0 {
			id = prefix + "action-Ä"
		}
		f.actions = append(f.actions, actions.Run{ID: id, Type: "command", Target: prefix + "asset", Status: "failed", CreatedAt: f.at})
		if i < 128 {
			f.audits = append(f.audits, audit.Event{ID: fmt.Sprintf("%saudit-%03d", prefix, i), Type: "action.failed", Target: prefix + "asset", ActorID: "operator", Timestamp: f.at, Details: map[string]any{"private": "omitted"}})
		}
	}
	f.incidents[0].GroupID = f.scope.GroupID
	f.incidents[0].PrimaryAssetID = prefix + "asset"
	f.incidents = append(f.incidents, incidents.Incident{ID: prefix + "0-closed", Status: "closed", OpenedAt: f.at, CreatedAt: f.at}, incidents.Incident{ID: prefix + "0-future", Status: "open", OpenedAt: f.at, CreatedAt: f.window.Through.Add(time.Second)})
	for i := 0; i < 131; i++ {
		id := fmt.Sprintf("%s0-foreign-%03d", prefix, i)
		f.actions = append(f.actions, actions.Run{ID: id, Type: "command", Target: prefix + "other", Status: "failed", CreatedAt: f.at})
		f.audits = append(f.audits, audit.Event{ID: id, Type: "foreign", Target: prefix + "other", Timestamp: f.at})
		bad := &alerts.FiringScope{Version: 2, AssetIDs: []string{prefix + "asset"}}
		f.alerts = append(f.alerts, alerts.AlertInstance{ID: id, RuleID: prefix + "rule", Fingerprint: id, Severity: "high", FirstFiredAt: &f.at, FiringScope: bad})
		if i < 6 {
			f.scope.AlertLinks = append(f.scope.AlertLinks, incidents.AlertLink{IncidentID: f.scope.IncidentID, AlertRuleID: prefix + "rule", AlertInstanceID: id, AlertFingerprint: id})
		}
	}
	f.actions = append(f.actions, actions.Run{ID: prefix + "0-success", Type: "command", Target: prefix + "asset", Status: "succeeded", CreatedAt: f.at}, actions.Run{ID: prefix + "0-old", Type: "command", Target: prefix + "asset", Status: "failed", CreatedAt: f.window.From.Add(-time.Second)})
	f.updates = []updates.Run{
		{ID: prefix + "update-match", PlanID: prefix + "plan", Status: "failed", CreatedAt: f.at, Results: []updates.RunResultEntry{{Target: prefix + "asset", Status: "succeeded"}, {Target: prefix + "other", Status: "failed"}, {Target: prefix + "asset-b", Status: "failed"}, {Target: prefix + "asset-b", Status: "failed"}}},
		{ID: prefix + "update-wrong", PlanID: prefix + "plan", Status: "failed", CreatedAt: f.at, Results: []updates.RunResultEntry{{Target: prefix + "asset", Status: "succeeded"}, {Target: prefix + "other", Status: "failed"}}},
		{ID: prefix + "update-malformed", PlanID: prefix + "plan", Status: "failed", CreatedAt: f.at},
	}
	for _, name := range []string{"asset", "group", "link", "legacy", "wrong-link", "never", "whitespace"} {
		row := alerts.AlertInstance{ID: prefix + "alert-" + name, RuleID: prefix + "rule", Fingerprint: name, Severity: "high", FirstFiredAt: &f.at}
		switch name {
		case "asset":
			row.FiringScope = &alerts.FiringScope{Version: 1, AssetIDs: []string{prefix + "other", prefix + "asset"}}
		case "group":
			row.FiringScope = &alerts.FiringScope{Version: 1, GroupIDs: []string{prefix + "group"}}
		case "link":
			row.FiringScope = &alerts.FiringScope{Version: 1}
		case "never":
			row.FirstFiredAt = nil
			row.FiringScope = &alerts.FiringScope{Version: 1, AssetIDs: []string{prefix + "asset"}}
		case "whitespace":
			row.FiringScope = &alerts.FiringScope{Version: 1, AssetIDs: []string{prefix + "asset"}, SyntheticCheckIDs: []string{"\u00a0bad"}}
		}
		f.alerts = append(f.alerts, row)
	}
	for _, name := range []string{"link", "legacy", "wrong-link"} {
		fingerprint := name
		if name == "wrong-link" {
			fingerprint = "different"
		}
		f.scope.AlertLinks = append(f.scope.AlertLinks, incidents.AlertLink{IncidentID: f.scope.IncidentID, AlertRuleID: prefix + "rule", AlertInstanceID: prefix + "alert-" + name, AlertFingerprint: fingerprint})
	}
	// Exact incident-target audit is allowed alongside scoped assets.
	f.audits[0].Target = f.scope.IncidentID
	return f
}

type correlationPageOwners struct {
	IncidentCorrelationPager
	ActionCorrelationPager
	UpdateCorrelationPager
	AlertCorrelationPager
	AuditCorrelationPager
}

func checkCorrelationPageContract(t *testing.T, ctx context.Context, f correlationPageFixture, o correlationPageOwners, mutateAfterFirst func()) {
	t.Helper()
	rows, cursor, done, err := o.ReadActiveIncidentCorrelationPage(ctx, f.window.Through, CorrelationCursor{})
	if err != nil || len(rows) != 128 || done || !cursor.Started {
		t.Fatalf("active first page: %d/%v/%v", len(rows), done, err)
	}
	tail, next, done, err := o.ReadActiveIncidentCorrelationPage(ctx, f.window.Through, cursor)
	if err != nil || len(tail) != 1 || !done || next.UpperID != cursor.UpperID || tail[0].ID <= cursor.AfterID {
		t.Fatalf("active continuation: %+v/%v/%v", tail, done, err)
	}
	inc, found, err := o.ReadIncidentCorrelation(ctx, f.scope.IncidentID)
	if err != nil || !found || inc.GroupID != f.scope.GroupID || inc.PrimaryAssetID != f.scope.AssetIDs[0] || !inc.OpenedAt.Equal(f.at) {
		t.Fatalf("exact current incident: %+v/%v", inc, err)
	}
	actions, first, done, err := o.ReadActionCorrelationPage(ctx, f.window, f.scope, CorrelationCursor{})
	if err != nil || len(actions) != 128 || done {
		t.Fatalf("filtered action page: %d/%v/%v", len(actions), done, err)
	}
	var got, want []string
	for _, row := range f.actions {
		if row.Status == "failed" && row.Target == f.scope.AssetIDs[0] && !row.CreatedAt.Before(f.window.From) {
			want = append(want, row.ID)
		}
	}
	sort.Strings(want)
	for _, row := range actions {
		got = append(got, row.ID)
		if row.Target != f.scope.AssetIDs[0] || row.Type != "command" {
			t.Fatal("action owner projection changed")
		}
	}
	mutateAfterFirst()
	remaining, next, done, err := o.ReadActionCorrelationPage(ctx, f.window, f.scope, first)
	if err != nil || len(remaining) != 1 || !done || next.UpperID != first.UpperID {
		t.Fatalf("action fixed range: %+v/%v/%v", remaining, done, err)
	}
	got = append(got, remaining[0].ID)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("byte ordered history differs: %d/%d", len(got), len(want))
	}
	retry, _, _, err := o.ReadActionCorrelationPage(ctx, f.window, f.scope, CorrelationCursor{Started: true, UpperID: first.UpperID})
	if err != nil || !reflect.DeepEqual(retry, actions) {
		t.Fatal("retry before effects did not preserve first range")
	}
	updates, _, done, err := o.ReadUpdateCorrelationPage(ctx, f.window, f.scope, CorrelationCursor{})
	if err != nil || !done || len(updates) != 1 || updates[0].Target != f.prefix+"asset-b" || updates[0].PlanID != f.prefix+"plan" {
		t.Fatalf("mixed update scope multiplied/broadened: %+v/%v", updates, err)
	}
	alerts, _, done, err := o.ReadAlertCorrelationPage(ctx, f.window, f.scope, CorrelationCursor{})
	got = nil
	for _, row := range alerts {
		got = append(got, row.ID)
		if row.Rejected || !row.FirstFiredAt.Equal(f.at) {
			t.Fatal("invalid fire evidence")
		}
		if row.FiringScope != nil && (len(row.FiringScope.AssetIDs) > 1 || len(row.FiringScope.GroupIDs) > 1 || len(row.FiringScope.SyntheticCheckIDs) > 0) {
			t.Fatal("unbounded witness projection")
		}
	}
	want = []string{f.prefix + "alert-asset", f.prefix + "alert-group", f.prefix + "alert-legacy", f.prefix + "alert-link"}
	if err != nil || !done || !reflect.DeepEqual(got, want) {
		t.Fatalf("saved scope/link/null/invalid filtering: %v/%v", got, err)
	}
	audits, auditCursor, done, err := o.ReadAuditCorrelationPage(ctx, f.window, f.scope, CorrelationCursor{})
	if err != nil || len(audits) != 128 || done {
		t.Fatalf("exact full audit page: %d/%v/%v", len(audits), done, err)
	}
	audits, next, done, err = o.ReadAuditCorrelationPage(ctx, f.window, f.scope, auditCursor)
	if err != nil || len(audits) != 0 || !done || next != auditCursor {
		t.Fatalf("full page needs true EOF: %d/%v/%v", len(audits), done, err)
	}
	empty := IncidentCorrelationScope{IncidentID: f.prefix + "unrelated"}
	if rows, _, _, err := o.ReadActionCorrelationPage(ctx, f.window, empty, CorrelationCursor{}); err != nil || len(rows) != 0 {
		t.Fatal("empty asset set became global")
	}
}

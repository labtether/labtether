package persistence

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/incidents"
)

// A critical rule that fired, recovered and fired again opened a second
// automatic issue (the dedup was per alert instance), and every automatic
// issue had an empty timeline because its alert fired before the issue
// opened. One rule keeps one open issue, and the timeline shows each firing
// and recovery by name.
func TestPostgresAutoIncidentOneOpenIssuePerRuleWithAlertTimeline(t *testing.T) {
	s := newTestPostgresStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	suffix := time.Now().UTC().UnixNano()
	owner := fmt.Sprintf("auto-issue-rule-%d", suffix)
	deviceID := fmt.Sprintf("auto-issue-device-%d", suffix)

	if _, err := s.UpsertAssetHeartbeat(assets.HeartbeatRequest{AssetID: deviceID, Name: "nas01", Source: "agent", Type: "host", Status: "online"}); err != nil {
		t.Fatalf("seed device: %v", err)
	}
	rule, err := s.CreateAlertRule(alerts.CreateRuleRequest{Name: "High CPU", Kind: alerts.RuleKindMetricThreshold,
		Severity: alerts.SeverityCritical, TargetScope: alerts.TargetScopeAsset, CreatedBy: owner})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		_, _ = s.pool.Exec(cleanup, `DELETE FROM incidents WHERE created_by=$1`, owner)
		_ = s.DeleteAlertRule(rule.ID)
		_ = s.DeleteAsset(deviceID)
	})

	fire := func() alerts.AlertInstance {
		t.Helper()
		inst, err := s.CreateAlertInstanceContext(ctx, alerts.CreateInstanceRequest{RuleID: rule.ID, Fingerprint: "fp-" + owner, Severity: alerts.SeverityCritical})
		if err != nil {
			t.Fatalf("create instance: %v", err)
		}
		if inst, err = s.FireAlertInstance(ctx, inst.ID, alerts.FiringScope{Version: 1, AssetIDs: []string{deviceID}}); err != nil {
			t.Fatalf("fire instance: %v", err)
		}
		return inst
	}
	open := func(inst alerts.AlertInstance) (incidents.Incident, bool) {
		t.Helper()
		inc, created, err := s.CreateAutoIncidentForAlertInstanceContext(ctx, inst.ID,
			incidents.CreateIncidentRequest{Title: "High CPU on nas01", Severity: incidents.SeverityCritical, CreatedBy: owner})
		if err != nil {
			t.Fatalf("auto incident for %s: %v", inst.ID, err)
		}
		return inc, created
	}
	resolve := func(inst alerts.AlertInstance) {
		t.Helper()
		if _, err := s.UpdateAlertInstanceStatusContext(ctx, inst.ID, alerts.InstanceStatusResolved); err != nil {
			t.Fatalf("resolve %s: %v", inst.ID, err)
		}
	}
	timeline := func(incidentID string) []string {
		t.Helper()
		events, err := s.ListIncidentEvents(incidentID, 50)
		if err != nil {
			t.Fatalf("timeline: %v", err)
		}
		out := make([]string, 0, len(events))
		for i := len(events) - 1; i >= 0; i-- {
			out = append(out, events[i].EventType+": "+events[i].Summary)
		}
		return out
	}

	first := fire()
	issue, created := open(first)
	if !created || issue.PrimaryAssetID != deviceID {
		t.Fatalf("first firing: created=%t primary asset %q, want a new issue for %s", created, issue.PrimaryAssetID, deviceID)
	}
	if got := timeline(issue.ID); len(got) != 1 || got[0] != "alert_fired: High CPU fired on nas01" {
		t.Fatalf("new issue timeline = %q, want the firing", got)
	}

	resolve(first)
	second := fire()
	if _, created := open(second); created {
		t.Fatal("second firing of the same rule opened another issue while the first is open")
	}
	var issues int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM incidents WHERE created_by=$1`, owner).Scan(&issues); err != nil || issues != 1 {
		t.Fatalf("open issues for the rule = %d (%v), want 1", issues, err)
	}
	want := []string{"alert_fired: High CPU fired on nas01", "alert_resolved: High CPU recovered on nas01", "alert_fired: High CPU fired on nas01"}
	if got := timeline(issue.ID); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("timeline after refire = %q, want %q", got, want)
	}

	// Once the issue is resolved, the next firing opens a new one.
	resolved := incidents.StatusResolved
	if _, err := s.UpdateIncident(issue.ID, incidents.UpdateIncidentRequest{Status: &resolved}); err != nil {
		t.Fatalf("resolve issue: %v", err)
	}
	resolve(second)
	if _, created := open(fire()); !created {
		t.Fatal("firing after the issue was resolved did not open a new issue")
	}
}

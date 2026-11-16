package alerting_test

import (
	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/hubapi/testutil/hubdeps"
	"github.com/labtether/labtether/internal/incidents"
	"github.com/labtether/labtether/internal/persistence"
	"testing"
	"time"
)

func TestMaybeAutoCreateIncidentUsesAtomicStore(t *testing.T) {
	sut := hubdeps.New(t)

	incidentCounter := &countingIncidentStore{IncidentStore: sut.IncidentStore}
	sut.IncidentStore = incidentCounter

	existing, err := sut.IncidentStore.CreateIncident(incidents.CreateIncidentRequest{
		Title:    "Existing Auto Incident",
		Severity: incidents.SeverityCritical,
		Source:   incidents.SourceAlertAuto,
	})
	if err != nil {
		t.Fatalf("create auto incident: %v", err)
	}
	if _, err := sut.IncidentStore.LinkIncidentAlert(existing.ID, incidents.LinkAlertRequest{
		AlertInstanceID: "inst-dup",
		LinkType:        incidents.LinkTypeTrigger,
	}); err != nil {
		t.Fatalf("link existing auto incident: %v", err)
	}

	sut.AlertingDeps().MaybeAutoCreateIncident(alerts.Rule{
		ID:       "rule-dup",
		Name:     "Duplicate CPU Saturation",
		Severity: alerts.SeverityCritical,
	}, alerts.AlertInstance{
		ID:        "inst-dup",
		StartedAt: time.Now().Add(-10 * time.Minute),
	})

	autoIncidents, err := sut.IncidentStore.ListIncidents(persistence.IncidentFilter{
		Limit:  10,
		Source: incidents.SourceAlertAuto,
	})
	if err != nil {
		t.Fatalf("list auto incidents: %v", err)
	}
	if len(autoIncidents) != 1 {
		t.Fatalf("expected duplicate auto incident creation to be skipped, incidents=%d", len(autoIncidents))
	}
	if incidentCounter.createAutoIncidentCalls != 1 || incidentCounter.hasAutoIncidentCalls != 0 {
		t.Fatalf("expected one atomic create and no separate lookup, got %d/%d", incidentCounter.createAutoIncidentCalls, incidentCounter.hasAutoIncidentCalls)
	}
	if incidentCounter.listIncidentAlertLinksCalls != 0 {
		t.Fatalf("expected no per-incident link scans, got %d", incidentCounter.listIncidentAlertLinksCalls)
	}
}

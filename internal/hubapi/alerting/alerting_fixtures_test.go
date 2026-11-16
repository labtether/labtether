package alerting_test

import (
	"context"
	"testing"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/hubapi/testutil/hubdeps"
	"github.com/labtether/labtether/internal/incidents"
	"github.com/labtether/labtether/internal/persistence"
)

// Helpers for the alerting tests moved from cmd/labtether.

// mustCreateAlertRule is cmd's helper of the same name over the Hub's store;
// cmd keeps its own for the tests that stay there.
func mustCreateAlertRule(t *testing.T, sut *hubdeps.Hub, req alerts.CreateRuleRequest) alerts.Rule {
	t.Helper()

	rule, err := sut.AlertStore.CreateAlertRule(req)
	if err != nil {
		t.Fatalf("failed to create alert rule: %v", err)
	}
	return rule
}

// countingIncidentStore and its methods moved unchanged from cmd's
// performance_hotspots_test.go and performance_alert_context_test.go; the
// one test that used them moved here. The method set decides which incident
// store contracts the evaluator sees, so all of them are here.
type countingIncidentStore struct {
	persistence.IncidentStore
	listIncidentAlertLinksCalls int
	hasAutoIncidentCalls        int
	createAutoIncidentCalls     int
}

func (c *countingIncidentStore) ListIncidentAlertLinks(incidentID string, limit int) ([]incidents.AlertLink, error) {
	c.listIncidentAlertLinksCalls++
	return c.IncidentStore.ListIncidentAlertLinks(incidentID, limit)
}

func (c *countingIncidentStore) HasAutoIncidentForAlertInstance(alertInstanceID string) (bool, error) {
	c.hasAutoIncidentCalls++
	if store, ok := c.IncidentStore.(interface {
		HasAutoIncidentForAlertInstance(alertInstanceID string) (bool, error)
	}); ok {
		return store.HasAutoIncidentForAlertInstance(alertInstanceID)
	}
	return false, nil
}

func (c *countingIncidentStore) HasAutoIncidentForAlertInstanceContext(ctx context.Context, id string) (bool, error) {
	c.hasAutoIncidentCalls++
	return c.IncidentStore.(persistence.AlertIncidentContextStore).HasAutoIncidentForAlertInstanceContext(ctx, id)
}

func (c *countingIncidentStore) CreateIncidentContext(ctx context.Context, req incidents.CreateIncidentRequest) (incidents.Incident, error) {
	return c.IncidentStore.(persistence.AlertIncidentContextStore).CreateIncidentContext(ctx, req)
}

func (c *countingIncidentStore) LinkIncidentAlertContext(ctx context.Context, id string, req incidents.LinkAlertRequest) (incidents.AlertLink, error) {
	return c.IncidentStore.(persistence.AlertIncidentContextStore).LinkIncidentAlertContext(ctx, id, req)
}

func (c *countingIncidentStore) CreateAutoIncidentForAlertInstanceContext(ctx context.Context, id string, req incidents.CreateIncidentRequest) (incidents.Incident, bool, error) {
	c.createAutoIncidentCalls++
	return c.IncidentStore.(persistence.AlertAutoIncidentContextStore).CreateAutoIncidentForAlertInstanceContext(ctx, id, req)
}

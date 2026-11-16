package alerting_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/groupmaintenance"
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

// mustCreateGroup and maintenanceOnlyGroupMaintenanceStore are copies of
// cmd's helpers (alerts_incidents_api_test.go, actions_updates_api_test.go),
// which many cmd tests still use: mustCreateGroup over the Hub, the store
// verbatim.

// mustCreateGroup creates a group via the HTTP API and returns its ID.
func mustCreateGroup(t *testing.T, sut *hubdeps.Hub, name, slug string) string {
	t.Helper()

	payload := []byte(`{"name":"` + name + `","slug":"` + slug + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/groups", bytes.NewReader(payload))
	rec := httptest.NewRecorder()
	sut.ResourcesDeps().HandleGroups(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 creating group, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Group struct {
			ID string `json:"id"`
		} `json:"group"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode group response: %v", err)
	}
	groupID := resp.Group.ID
	t.Cleanup(func() {
		if sut == nil || sut.GroupStore == nil || groupID == "" {
			return
		}
		if err := sut.GroupStore.DeleteGroup(groupID); err != nil {
			t.Logf("group cleanup warning: failed to delete group %q: %v", groupID, err)
		}
	})
	return groupID
}

type maintenanceOnlyGroupMaintenanceStore struct {
	windows map[string][]groupmaintenance.MaintenanceWindow
}

func (m *maintenanceOnlyGroupMaintenanceStore) CreateGroupMaintenanceWindow(groupID string, req groupmaintenance.CreateMaintenanceWindowRequest) (groupmaintenance.MaintenanceWindow, error) {
	window := groupmaintenance.MaintenanceWindow{
		ID:             "mw-" + groupID,
		GroupID:        groupID,
		Name:           req.Name,
		StartAt:        req.StartAt.UTC(),
		EndAt:          req.EndAt.UTC(),
		SuppressAlerts: req.SuppressAlerts,
		BlockActions:   req.BlockActions,
		BlockUpdates:   req.BlockUpdates,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}
	if m.windows == nil {
		m.windows = make(map[string][]groupmaintenance.MaintenanceWindow)
	}
	m.windows[groupID] = append(m.windows[groupID], window)
	return window, nil
}

func (m *maintenanceOnlyGroupMaintenanceStore) GetGroupMaintenanceWindow(groupID, windowID string) (groupmaintenance.MaintenanceWindow, bool, error) {
	for _, window := range m.windows[groupID] {
		if window.ID == windowID {
			return window, true, nil
		}
	}
	return groupmaintenance.MaintenanceWindow{}, false, nil
}

func (m *maintenanceOnlyGroupMaintenanceStore) ListGroupMaintenanceWindows(groupID string, activeAt *time.Time, limit int) ([]groupmaintenance.MaintenanceWindow, error) {
	windows := append([]groupmaintenance.MaintenanceWindow(nil), m.windows[groupID]...)
	if activeAt == nil {
		return windows, nil
	}
	filtered := make([]groupmaintenance.MaintenanceWindow, 0, len(windows))
	for _, window := range windows {
		if !window.StartAt.After(activeAt.UTC()) && !window.EndAt.Before(activeAt.UTC()) {
			filtered = append(filtered, window)
		}
	}
	return filtered, nil
}

func (m *maintenanceOnlyGroupMaintenanceStore) UpdateGroupMaintenanceWindow(groupID, windowID string, req groupmaintenance.UpdateMaintenanceWindowRequest) (groupmaintenance.MaintenanceWindow, error) {
	return groupmaintenance.MaintenanceWindow{}, errors.New("not implemented")
}

func (m *maintenanceOnlyGroupMaintenanceStore) DeleteGroupMaintenanceWindow(groupID, windowID string) error {
	return errors.New("not implemented")
}

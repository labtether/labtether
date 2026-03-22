package alerting

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labtether/labtether/internal/alerts"
)

func TestHandleAlertInstanceActionsDeleteRemovesInstance(t *testing.T) {
	deps := newTestAlertingDeps(t)

	inst, err := deps.AlertInstanceStore.CreateAlertInstance(alerts.CreateInstanceRequest{
		RuleID:   "rule-delete-test",
		Severity: alerts.SeverityHigh,
	})
	if err != nil {
		t.Fatalf("failed to create alert instance: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/alerts/instances/"+inst.ID, nil)
	rec := httptest.NewRecorder()
	deps.HandleAlertInstanceActions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 deleting alert instance, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode delete response: %v", err)
	}
	if payload.Status != "deleted" {
		t.Fatalf("expected deleted status response, got %q", payload.Status)
	}

	_, ok, err := deps.AlertInstanceStore.GetAlertInstance(inst.ID)
	if err != nil {
		t.Fatalf("failed to load alert instance after delete: %v", err)
	}
	if ok {
		t.Fatalf("expected alert instance %q to be removed", inst.ID)
	}
}

func TestHandleAlertInstanceActionsDeleteNotFound(t *testing.T) {
	deps := newTestAlertingDeps(t)

	req := httptest.NewRequest(http.MethodDelete, "/alerts/instances/ainst-missing", nil)
	rec := httptest.NewRecorder()
	deps.HandleAlertInstanceActions(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 deleting missing alert instance, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleAlertInstanceActionsRejectsUnsupportedMethodOnInstance(t *testing.T) {
	deps := newTestAlertingDeps(t)

	inst, err := deps.AlertInstanceStore.CreateAlertInstance(alerts.CreateInstanceRequest{
		RuleID:   "rule-method-test",
		Severity: alerts.SeverityMedium,
	})
	if err != nil {
		t.Fatalf("failed to create alert instance: %v", err)
	}

	req := httptest.NewRequest(http.MethodPatch, "/alerts/instances/"+inst.ID, nil)
	rec := httptest.NewRecorder()
	deps.HandleAlertInstanceActions(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for unsupported method, got %d body=%s", rec.Code, rec.Body.String())
	}
}

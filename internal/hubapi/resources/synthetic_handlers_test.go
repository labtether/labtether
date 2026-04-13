package resources

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labtether/labtether/internal/synthetic"
)

func TestHandleSyntheticCheckActionsResultsMissingCheckReturnsNotFound(t *testing.T) {
	deps := newTestResourcesDeps(t)
	req := httptest.NewRequest(http.MethodGet, "/synthetic-checks/missing/results", nil)
	rec := httptest.NewRecorder()

	deps.HandleSyntheticCheckActions(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleSyntheticCheckActionsRejectsNonPositiveIntervalOnUpdate(t *testing.T) {
	deps := newTestResourcesDeps(t)
	check, err := deps.SyntheticStore.CreateSyntheticCheck(synthetic.CreateCheckRequest{
		Name:      "Homepage",
		CheckType: synthetic.CheckTypeHTTP,
		Target:    "https://example.com",
	})
	if err != nil {
		t.Fatalf("failed to create check: %v", err)
	}

	payload := []byte(`{"interval_seconds":0}`)
	req := httptest.NewRequest(http.MethodPatch, "/synthetic-checks/"+check.ID, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	deps.HandleSyntheticCheckActions(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}

	stored, ok, err := deps.SyntheticStore.GetSyntheticCheck(check.ID)
	if err != nil {
		t.Fatalf("failed to reload check: %v", err)
	}
	if !ok {
		t.Fatalf("expected check to exist")
	}
	if stored.IntervalSeconds <= 0 {
		t.Fatalf("expected interval to remain positive, got %d", stored.IntervalSeconds)
	}
}

func TestHandleSyntheticCheckActionsResultsReturnsExistingCheckResults(t *testing.T) {
	deps := newTestResourcesDeps(t)
	check, err := deps.SyntheticStore.CreateSyntheticCheck(synthetic.CreateCheckRequest{
		Name:      "Homepage",
		CheckType: synthetic.CheckTypeHTTP,
		Target:    "https://example.com",
	})
	if err != nil {
		t.Fatalf("failed to create check: %v", err)
	}
	if _, err := deps.SyntheticStore.RecordSyntheticResult(check.ID, synthetic.Result{Status: synthetic.ResultStatusOK}); err != nil {
		t.Fatalf("failed to record result: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/synthetic-checks/"+check.ID+"/results", nil)
	rec := httptest.NewRecorder()

	deps.HandleSyntheticCheckActions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Results []synthetic.Result `json:"results"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode results: %v", err)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Results))
	}
}

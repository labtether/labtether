package updatespkg

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/updates"
)

func restrictedUpdateRequest(method, path string, body []byte, allowed ...string) *http.Request {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	return req.WithContext(apiv2.ContextWithAllowedAssets(context.Background(), allowed))
}

func newUpdateAuthorizationDeps() *Deps {
	return &Deps{
		UpdateStore: persistence.NewMemoryUpdateStore(),
		EnforceRateLimit: func(http.ResponseWriter, *http.Request, string, int, time.Duration) bool {
			return true
		},
	}
}

func TestRestrictedUpdateCollectionsFilterPlansAndRuns(t *testing.T) {
	d := newUpdateAuthorizationDeps()
	allowedPlan, err := d.UpdateStore.CreateUpdatePlan(updates.CreatePlanRequest{Name: "allowed", Targets: []string{"asset-a"}})
	if err != nil {
		t.Fatal(err)
	}
	secretPlan, err := d.UpdateStore.CreateUpdatePlan(updates.CreatePlanRequest{Name: "secret", Targets: []string{"asset-b"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.UpdateStore.CreateUpdateRun(allowedPlan, updates.ExecutePlanRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.UpdateStore.CreateUpdateRun(secretPlan, updates.ExecutePlanRequest{}); err != nil {
		t.Fatal(err)
	}

	planRec := httptest.NewRecorder()
	d.HandleUpdatePlans(planRec, restrictedUpdateRequest(http.MethodGet, "/updates/plans", nil, "asset-a"))
	if planRec.Code != http.StatusOK {
		t.Fatalf("plans: expected 200, got %d body=%s", planRec.Code, planRec.Body.String())
	}
	var plansResponse struct {
		Plans []updates.Plan `json:"plans"`
	}
	if err := json.Unmarshal(planRec.Body.Bytes(), &plansResponse); err != nil {
		t.Fatal(err)
	}
	if len(plansResponse.Plans) != 1 || plansResponse.Plans[0].ID != allowedPlan.ID {
		t.Fatalf("unexpected filtered plans: %#v", plansResponse.Plans)
	}

	runRec := httptest.NewRecorder()
	d.HandleUpdateRuns(runRec, restrictedUpdateRequest(http.MethodGet, "/updates/runs", nil, "asset-a"))
	if runRec.Code != http.StatusOK {
		t.Fatalf("runs: expected 200, got %d body=%s", runRec.Code, runRec.Body.String())
	}
	var runsResponse struct {
		Runs []updates.Run `json:"runs"`
	}
	if err := json.Unmarshal(runRec.Body.Bytes(), &runsResponse); err != nil {
		t.Fatal(err)
	}
	if len(runsResponse.Runs) != 1 || runsResponse.Runs[0].PlanID != allowedPlan.ID {
		t.Fatalf("unexpected filtered runs: %#v", runsResponse.Runs)
	}
}

func TestRestrictedUpdatePlanWriteAndObjectReadFailClosed(t *testing.T) {
	d := newUpdateAuthorizationDeps()
	secretPlan, err := d.UpdateStore.CreateUpdatePlan(updates.CreatePlanRequest{Name: "secret", Targets: []string{"asset-b"}})
	if err != nil {
		t.Fatal(err)
	}

	readRec := httptest.NewRecorder()
	d.HandleUpdatePlanActions(readRec, restrictedUpdateRequest(http.MethodGet, "/updates/plans/"+secretPlan.ID, nil, "asset-a"))
	if readRec.Code != http.StatusForbidden {
		t.Fatalf("read: expected 403, got %d body=%s", readRec.Code, readRec.Body.String())
	}

	createRec := httptest.NewRecorder()
	d.HandleUpdatePlans(createRec, restrictedUpdateRequest(
		http.MethodPost,
		"/updates/plans",
		[]byte(`{"name":"mixed","targets":["asset-a","asset-b"]}`),
		"asset-a",
	))
	if createRec.Code != http.StatusForbidden {
		t.Fatalf("create: expected 403, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	plans, err := d.UpdateStore.ListUpdatePlans(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 1 {
		t.Fatalf("forbidden create reached store: got %d plans", len(plans))
	}
}

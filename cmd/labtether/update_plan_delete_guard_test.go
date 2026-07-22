package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/updates"
)

func TestV2UpdatePlanDeleteReturnsDeterministicConflictForActiveRun(t *testing.T) {
	sut := newTestAPIServer(t)
	store := sut.updateStore.(*persistence.MemoryUpdateStore)
	plan, err := store.CreateUpdatePlan(updates.CreatePlanRequest{
		Name:    "Active plan",
		Targets: []string{"lab-host-01"},
	})
	if err != nil {
		t.Fatalf("create update plan: %v", err)
	}
	run, err := store.CreateUpdateRun(plan, updates.ExecutePlanRequest{ActorID: "qa"})
	if err != nil {
		t.Fatalf("create update run: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/v2/updates/plans/"+plan.ID, nil)
	req = req.WithContext(contextWithScopes(req.Context(), []string{"updates:write"}))
	rec := httptest.NewRecorder()
	sut.handleV2UpdatePlanActions(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s, want 409", rec.Code, rec.Body.String())
	}
	var response struct {
		Error   string `json:"error"`
		Message string `json:"message"`
		Status  int    `json:"status"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode conflict response: %v", err)
	}
	if response.Error != "update_plan_active" || response.Status != http.StatusConflict {
		t.Fatalf("conflict response=%+v", response)
	}
	if response.Message != "Update plan cannot be deleted while runs are queued or running." {
		t.Fatalf("message=%q", response.Message)
	}
	if _, ok, err := store.GetUpdatePlan(plan.ID); err != nil || !ok {
		t.Fatalf("active plan was removed: ok=%t err=%v", ok, err)
	}
	if _, ok, err := store.GetUpdateRun(run.ID); err != nil || !ok {
		t.Fatalf("active run was removed: ok=%t err=%v", ok, err)
	}
}

func TestV2UpdatePlanDeleteCascadesTerminalRunAndReturnsExactEvidence(t *testing.T) {
	sut := newTestAPIServer(t)
	store := sut.updateStore.(*persistence.MemoryUpdateStore)
	plan, err := store.CreateUpdatePlan(updates.CreatePlanRequest{
		Name:    "Completed plan",
		Targets: []string{"lab-host-01"},
	})
	if err != nil {
		t.Fatalf("create update plan: %v", err)
	}
	run, err := store.CreateUpdateRun(plan, updates.ExecutePlanRequest{ActorID: "qa"})
	if err != nil {
		t.Fatalf("create update run: %v", err)
	}
	if err := store.ApplyUpdateResult(updates.Result{RunID: run.ID, Status: updates.StatusSucceeded}); err != nil {
		t.Fatalf("complete update run: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/v2/updates/plans/"+plan.ID, nil)
	req = req.WithContext(contextWithScopes(req.Context(), []string{"updates:write"}))
	rec := httptest.NewRecorder()
	sut.handleV2UpdatePlanActions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s, want 200", rec.Code, rec.Body.String())
	}
	var response struct {
		Data struct {
			Deleted bool   `json:"deleted"`
			PlanID  string `json:"plan_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode delete response: %v", err)
	}
	if !response.Data.Deleted || response.Data.PlanID != plan.ID {
		t.Fatalf("delete evidence=%+v, want plan %q", response.Data, plan.ID)
	}
	if _, ok, err := store.GetUpdatePlan(plan.ID); err != nil || ok {
		t.Fatalf("deleted plan remains: ok=%t err=%v", ok, err)
	}
	if _, ok, err := store.GetUpdateRun(run.ID); err != nil || ok {
		t.Fatalf("terminal run was orphaned: ok=%t err=%v", ok, err)
	}
}
